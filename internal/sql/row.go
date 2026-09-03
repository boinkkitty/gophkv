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

var ErrBadKey = errors.New("bad key")

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

// DecodeKey decodes a key buffer into the row's primary-key columns.
func (row Row) DecodeKey(schema *Schema, key []byte) error {
	utils.Check(len(row) == len(schema.Cols))
	if len(key) < len(schema.Table)+1 {
		return ErrBadKey
	}
	if string(key[:len(schema.Table)+1]) != schema.Table+"\x00" {
		return ErrBadKey
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
