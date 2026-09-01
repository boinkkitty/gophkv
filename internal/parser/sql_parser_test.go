package parser

import (
	"testing"

	"github.com/boinkkitty/gophkv/internal/table"
	"github.com/stretchr/testify/assert"
)

func TestParseName(t *testing.T) {
	p := NewParser(" a b0 _0_ 123 ")
	name, ok := p.tryName()
	assert.True(t, ok && name == "a")
	name, ok = p.tryName()
	assert.True(t, ok && name == "b0")
	name, ok = p.tryName()
	assert.True(t, ok && name == "_0_")
	_, ok = p.tryName()
	assert.False(t, ok)
}

func TestParseKeyword(t *testing.T) {
	p := NewParser(" select  HELLO ")
	assert.False(t, p.tryKeyword("sel"))
	assert.True(t, p.tryKeyword("SELECT"))
	assert.True(t, p.tryKeyword("hello") && p.isEnd())
}

func testParseValue(t *testing.T, s string, ref table.Cell) {
	p := NewParser(s)
	out := table.Cell{}
	err := p.parseValue(&out)
	assert.Nil(t, err)
	assert.True(t, p.isEnd())
	assert.Equal(t, ref, out)
}

func TestParseValue(t *testing.T) {
	testParseValue(t, " -123 ", table.Cell{Type: table.TypeI64, I64: -123})
	testParseValue(t, ` 'abc\'\"d' `, table.Cell{Type: table.TypeStr, Str: []byte("abc'\"d")})
	testParseValue(t, ` "abc\'\"d" `, table.Cell{Type: table.TypeStr, Str: []byte("abc'\"d")})
}
