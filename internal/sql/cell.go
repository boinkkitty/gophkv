package sql

import (
	"encoding/binary"
	"errors"
	"slices"
)

type CellType uint8

const (
	TypeI64 CellType = 1
	TypeStr CellType = 2
)

type Cell struct {
	Type CellType
	I64  int64
	Str  []byte
}

var ErrMoreData = errors.New("expect more data")

// Cell binary formats:
//
// Value encoding:
//   TypeI64: [8 bytes i64, little-endian]
//   TypeStr: [4 bytes length, little-endian][N raw string bytes]
//
// Key encoding:
//   TypeI64: [8 bytes i64, big-endian, sign bit flipped]
//   TypeStr: [escaped string bytes][0x00 terminator, 1 byte]
//
// String key escaping:
//   0x00 -> 0x01 0x01
//   0x01 -> 0x01 0x02
//   else -> unchanged
//
// The final 0x00 byte appears only as the string terminator, so adjacent key
// fields can be decoded unambiguously.

// EncodeVal appends the cell's binary value form to toAppend.
func (cell *Cell) EncodeVal(toAppend []byte) []byte {
	switch cell.Type {
	case TypeI64:
		return binary.LittleEndian.AppendUint64(toAppend, uint64(cell.I64))
	case TypeStr:
		toAppend = binary.LittleEndian.AppendUint32(toAppend, uint32(len(cell.Str)))
		return append(toAppend, cell.Str...)
	default:
		panic("unreachable")
	}
}

// DecodeVal fills the cell from a value buffer and returns any remaining bytes.
func (cell *Cell) DecodeVal(data []byte) ([]byte, error) {
	switch cell.Type {
	case TypeI64:
		if len(data) < 8 {
			return data, ErrMoreData
		}
		cell.I64 = int64(binary.LittleEndian.Uint64(data[:8]))
		return data[8:], nil
	case TypeStr:
		if len(data) < 4 {
			return data, ErrMoreData
		}
		size := int(binary.LittleEndian.Uint32(data[:4]))
		if len(data) < 4+size {
			return data, ErrMoreData
		}
		cell.Str = slices.Clone(data[4 : 4+size])
		return data[4+size:], nil
	default:
		panic("unreachable")
	}
}

// encodeStrKey encodes a string for ordered key storage.
//
// Raw strings cannot be stored directly in a composite key because key decoding
// needs a boundary between adjacent string fields. The format here uses a
// trailing 0x00 byte as the terminator, like a C string.
//
// That creates one problem: if the original string itself contains 0x00, the
// decoder would stop too early. To avoid that, the payload removes literal
// 0x00 bytes by escaping them with 0x01:
//
//	0x00 -> 0x01 0x01
//
// The escape byte must also be escaped so the decoder can distinguish a literal
// escape from an escape prefix:
//
//	0x01 -> 0x01 0x02
//
// All other bytes are copied as-is, then a final 0x00 terminator is appended.
// This encoding is reversible and preserves lexicographic order.
func encodeStrKey(toAppend []byte, input []byte) []byte {
	for _, ch := range input {
		if ch == 0x00 || ch == 0x01 {
			toAppend = append(toAppend, 0x01, ch+1)
		} else {
			toAppend = append(toAppend, ch)
		}
	}
	return append(toAppend, 0x00)
}

// decodeStrKey decodes one escaped, null-terminated key string from data.
//
// It reads until an unescaped 0x00 terminator. When 0x01 is seen, the next byte
// must be:
//
//	0x01 -> original byte 0x00
//	0x02 -> original byte 0x01
//
// Any other byte after 0x01 is invalid. The returned rest slice begins
// immediately after the terminating 0x00, which lets row key decoding continue
// with the next primary-key field.
func decodeStrKey(data []byte) ([]byte, []byte, error) {
	out := make([]byte, 0)
	escape := false
	for i, ch := range data {
		if escape {
			if ch != 0x01 && ch != 0x02 {
				return nil, data, errors.New("bad escape")
			}
			out = append(out, ch-1)
			escape = false
		} else if ch == 0x00 {
			return out, data[i+1:], nil
		} else if ch == 0x01 {
			escape = true
		} else {
			out = append(out, ch)
		}
	}
	return nil, data, errors.New("string is not ended")
}

// EncodeKey appends the cell's order-preserving key form to toAppend.
func (cell *Cell) EncodeKey(toAppend []byte) []byte {
	switch cell.Type {
	case TypeI64:
		// XOR - flip MSB
		return binary.BigEndian.AppendUint64(toAppend, uint64(cell.I64)^(1<<63))
	case TypeStr:
		return encodeStrKey(toAppend, cell.Str)
	default:
		panic("unreachable")
	}
}

// DecodeKey fills the cell from a key buffer and returns any remaining bytes.
func (cell *Cell) DecodeKey(data []byte) ([]byte, error) {
	switch cell.Type {
	case TypeI64:
		if len(data) < 8 {
			return data, ErrMoreData
		}
		cell.I64 = int64(binary.BigEndian.Uint64(data[:8]) ^ (1 << 63))
		return data[8:], nil
	case TypeStr:
		str, rest, err := decodeStrKey(data)
		if err != nil {
			return rest, err
		}
		cell.Str = str
		return rest, nil
	default:
		panic("unreachable")
	}
}
