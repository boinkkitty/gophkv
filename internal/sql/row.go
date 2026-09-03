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
//
// Each primary-key field is encoded by Cell.EncodeKey in the exact order listed
// in schema.PKey.
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
	key := make([]byte, 0)
	key = append(key, []byte(schema.Table)...)
	key = append(key, 0x00)
	utils.Check(len(row) == len(schema.Cols))
	for _, idx := range schema.PKey {
		value := row[idx]
		utils.Check(value.Type == schema.Cols[idx].Type)
		key = row[idx].EncodeKey(key)
	}
	return key
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
		rest, err := row[idx].DecodeKey(key)
		if err != nil {
			return err
		}
		key = rest
	}

	if len(key) != 0 {
		return errors.New("trailing garbage")
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
