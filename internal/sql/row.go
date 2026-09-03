package sql

import (
	"errors"
	"slices"

	"github.com/boinkkitty/gophkv/internal/utils"
)

type Schema struct {
	Table string
	Cols  []Column
	PKey  []int // indexes of primary key columns
}

type Column struct {
	Name string
	Type CellType
}

type Row []Cell

// Row binary formats:
//
// Key:
//   [table name bytes]
//   [0x00 table terminator, 1 byte]
//   [primary-key field 1 bytes]
//   [primary-key field 2 bytes]
//   ...
//   [primary-key field N bytes]
//   [0x00 row-key terminator, 1 byte]
//
// Each primary-key field is encoded by Cell.EncodeKey in the exact order listed
// in schema.PKey. The table-name `0x00` and final row-key `0x00` are boundary
// markers: they separate the table prefix from the primary-key payload and mark
// the end of a fully encoded row key.
//
// Range scans sometimes encode only a key prefix instead of a full row key.
// EncodeKeyPrefix optionally appends `0xFF` as a positive upper-bound marker.
// Because encoded key bytes never need to exceed `0xFF`, that suffix sorts after
// any real continuation of the same prefix and acts like "+infinity" for that
// prefix.
//
// Lower-bound prefix shape:
//   [table name bytes][0x00 table terminator][type][field]...
//
// Upper-bound prefix shape:
//   [table name bytes][0x00 table terminator][type][field]...[0xFF]
//
// The lower bound omits the final row-key terminator because it is just a scan
// starting point, not a full stored row key. The upper bound adds `0xFF` so it
// sorts after every real key sharing the same prefix.
//
// Value:
//   [non-primary-key field 1 bytes]
//   [non-primary-key field 2 bytes]
//   ...
//   [non-primary-key field N bytes]
//
// Each non-primary-key field is encoded by Cell.EncodeVal in schema column
// order. Integer values use little-endian in the value payload. Integer keys
// use big-endian with the sign bit flipped so byte ordering matches numeric
// ordering.

// NewRow allocates a row sized to the schema's column count.
func (schema *Schema) NewRow() Row {
	return make(Row, len(schema.Cols))
}

// EncodeKey encodes the primary-key columns into the row key.
func (row Row) EncodeKey(schema *Schema) []byte {
	utils.Check(len(row) == len(schema.Cols))
	key := append([]byte(schema.Table), 0x00)
	for _, idx := range schema.PKey {
		cell := row[idx]
		utils.Check(cell.Type == schema.Cols[idx].Type)
		key = append(key, byte(cell.Type))
		key = cell.EncodeKey(key)
	}
	return append(key, 0x00)
}

// EncodeVal encodes the non-primary-key columns into the row value.
func (row Row) EncodeVal(schema *Schema) []byte {
	utils.Check(len(row) == len(schema.Cols))
	val := make([]byte, 0)
	for idx, value := range row {
		if !slices.Contains(schema.PKey, idx) {
			utils.Check(value.Type == schema.Cols[idx].Type)
			val = row[idx].EncodeVal(val)
		}
	}
	return val
}

// EncodeKeyPrefix encodes a partial primary-key prefix for range bounds.
// When positive is true, it appends 0xFF so the bound sorts after every real
// key with the same prefix.
func EncodeKeyPrefix(schema *Schema, prefix []Cell, positive bool) []byte {
	key := append([]byte(schema.Table), 0x00)
	for i, cell := range prefix {
		utils.Check(cell.Type == schema.Cols[schema.PKey[i]].Type)
		key = append(key, byte(cell.Type))
		key = cell.EncodeKey(key)
	}
	if positive {
		key = append(key, 0xff) // +infinity
	}
	return key
}

var ErrOutOfRange = errors.New("out of range")

// DecodeKey decodes a key buffer into the row's primary-key columns.
func (row Row) DecodeKey(schema *Schema, key []byte) error {
	utils.Check(len(row) == len(schema.Cols))
	if len(key) < len(schema.Table)+1 {
		return ErrOutOfRange
	}
	if string(key[:len(schema.Table)+1]) != schema.Table+"\x00" {
		return ErrOutOfRange
	}
	key = key[len(schema.Table)+1:]

	for _, idx := range schema.PKey {
		row[idx] = Cell{Type: schema.Cols[idx].Type}
		if !(len(key) > 0 && key[0] == byte(row[idx].Type)) {
			return errors.New("bad key")
		}
		key = key[1:]
		var err error
		if key, err = row[idx].DecodeKey(key); err != nil {
			return err
		}
	}
	if !(len(key) == 1 && key[0] == 0x00) {
		return errors.New("bad key")
	}
	return nil
}

// DecodeVal decodes a value buffer into the row's non-primary-key columns.
func (row Row) DecodeVal(schema *Schema, val []byte) error {
	utils.Check(len(row) == len(schema.Cols))
	for idx, col := range schema.Cols {
		if slices.Contains(schema.PKey, idx) {
			continue
		}
		row[idx] = Cell{Type: col.Type}
		rest, err := row[idx].DecodeVal(val)
		if err != nil {
			return err
		}
		val = rest
	}

	if len(val) != 0 {
		return errors.New("trailing garbage")
	}
	return nil
}
