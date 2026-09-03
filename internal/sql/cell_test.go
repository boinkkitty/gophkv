package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTableCell verifies cell encoding and decoding for supported types.
func TestTableCell(t *testing.T) {
	cell := Cell{Type: TypeI64, I64: -2}
	data := []byte{254, 255, 255, 255, 255, 255, 255, 255}
	assert.Equal(t, data, cell.Encode(nil))
	decoded := Cell{Type: TypeI64}
	rest, err := decoded.Decode(data)
	assert.Nil(t, err)
	assert.Len(t, rest, 0)
	assert.Equal(t, cell, decoded)

	cell = Cell{Type: TypeStr, Str: []byte("asdf")}
	data = []byte{4, 0, 0, 0, 'a', 's', 'd', 'f'}
	assert.Equal(t, data, cell.Encode(nil))
	decoded = Cell{Type: TypeStr}
	rest, err = decoded.Decode(data)
	assert.Nil(t, err)
	assert.Len(t, rest, 0)
	assert.Equal(t, cell, decoded)
}
