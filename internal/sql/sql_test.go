package sql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	p = NewParser(" select  HELLO ")
	assert.False(t, p.tryKeyword("select", "hi"))
	assert.True(t, p.tryKeyword("select", "hello") && p.isEnd())
}

func testParseValue(t *testing.T, s string, ref Cell) {
	p := NewParser(s)
	out := Cell{}
	err := p.parseValue(&out)
	assert.Nil(t, err)
	assert.True(t, p.isEnd())
	assert.Equal(t, ref, out)
}

func TestParseValue(t *testing.T) {
	testParseValue(t, " -123 ", Cell{Type: TypeI64, I64: -123})
	testParseValue(t, ` 'abc\'\"d' `, Cell{Type: TypeStr, Str: []byte("abc'\"d")})
	testParseValue(t, ` "abc\'\"d" `, Cell{Type: TypeStr, Str: []byte("abc'\"d")})
}

func testParseStmt(t *testing.T, s string, ref interface{}) {
	p := NewParser(s)
	out, err := p.parseStmt()
	assert.Nil(t, err)
	assert.True(t, p.isEnd())
	assert.Equal(t, ref, out)
}

func TestParseStmt(t *testing.T) {
	var stmt interface{}
	s := "select a from t where c=1;"
	stmt = &StmtSelect{
		Table: "t",
		Cols:  []interface{}{"a"},
		Cond:  &ExprBinOp{op: OP_EQ, left: "c", right: &Cell{Type: TypeI64, I64: 1}},
	}
	testParseStmt(t, s, stmt)

	s = "select a,b_02 from T where c=1 and d='e';"
	stmt = &StmtSelect{
		Table: "T",
		Cols:  []interface{}{"a", "b_02"},
		Cond: &ExprBinOp{op: OP_AND,
			left:  &ExprBinOp{op: OP_EQ, left: "c", right: &Cell{Type: TypeI64, I64: 1}},
			right: &ExprBinOp{op: OP_EQ, left: "d", right: &Cell{Type: TypeStr, Str: []byte("e")}},
		},
	}
	testParseStmt(t, s, stmt)

	s = "select a, b_02 from T where c = 1 and d = 'e' ; "
	testParseStmt(t, s, stmt)

	s = "create table t (a string, b int64, primary key (b));"
	stmt = &StmtCreatTable{
		Table: "t",
		Cols:  []Column{{Name: "a", Type: TypeStr}, {Name: "b", Type: TypeI64}},
		PKey:  []string{"b"},
	}
	testParseStmt(t, s, stmt)

	s = "insert into t values (1, 'hi');"
	stmt = &StmtInsert{
		Table: "t",
		Value: []Cell{{Type: TypeI64, I64: 1}, {Type: TypeStr, Str: []byte("hi")}},
	}
	testParseStmt(t, s, stmt)

	s = "update t set a = 1, b = 2 where c = 3 and d = 4;"
	stmt = &StmtUpdate{
		Table: "t",
		Value: []ExprAssign{
			{column: "a", expr: &Cell{Type: TypeI64, I64: 1}},
			{column: "b", expr: &Cell{Type: TypeI64, I64: 2}},
		},
		Cond: &ExprBinOp{op: OP_AND,
			left:  &ExprBinOp{op: OP_EQ, left: "c", right: &Cell{Type: TypeI64, I64: 3}},
			right: &ExprBinOp{op: OP_EQ, left: "d", right: &Cell{Type: TypeI64, I64: 4}},
		},
	}
	testParseStmt(t, s, stmt)

	s = "delete from t where c = 3 and d = 4;"
	stmt = &StmtDelete{
		Table: "t",
		Cond: &ExprBinOp{op: OP_AND,
			left:  &ExprBinOp{op: OP_EQ, left: "c", right: &Cell{Type: TypeI64, I64: 3}},
			right: &ExprBinOp{op: OP_EQ, left: "d", right: &Cell{Type: TypeI64, I64: 4}},
		},
	}
	testParseStmt(t, s, stmt)

	s = "delete from t where (c, d) >= (3, 4);"
	stmt = &StmtDelete{
		Table: "t",
		Cond: &ExprBinOp{op: OP_GE,
			left:  &ExprTuple{Kids: []interface{}{"c", "d"}},
			right: &ExprTuple{Kids: []interface{}{&Cell{Type: TypeI64, I64: 3}, &Cell{Type: TypeI64, I64: 4}}},
		},
	}
	testParseStmt(t, s, stmt)
}

func testParseExpr(t *testing.T, s string, expr interface{}) {
	p := NewParser(s)
	out, err := p.parseExpr()
	require.Nil(t, err)
	assert.Equal(t, expr, out)
	assert.True(t, p.isEnd())
}

func TestParseExpr(t *testing.T) {
	var expr interface{}

	testParseExpr(t, "a", "a")
	testParseExpr(t, "(a)", "a")
	testParseExpr(t, "1", &Cell{Type: TypeI64, I64: 1})

	s := "a + 1"
	expr = &ExprBinOp{op: OP_ADD, left: "a", right: &Cell{Type: TypeI64, I64: 1}}
	testParseExpr(t, s, expr)

	s = "a + 1 - b"
	expr = &ExprBinOp{op: OP_SUB,
		left:  &ExprBinOp{op: OP_ADD, left: "a", right: &Cell{Type: TypeI64, I64: 1}},
		right: "b",
	}
	testParseExpr(t, s, expr)

	s = "a + b * c"
	expr = &ExprBinOp{op: OP_ADD,
		left:  "a",
		right: &ExprBinOp{op: OP_MUL, left: "b", right: "c"},
	}
	testParseExpr(t, s, expr)

	s = "(a * b)"
	expr = &ExprBinOp{op: OP_MUL, left: "a", right: "b"}
	testParseExpr(t, s, expr)

	s = "(a + b) / c"
	expr = &ExprBinOp{op: OP_DIV,
		left:  &ExprBinOp{op: OP_ADD, left: "a", right: "b"},
		right: "c",
	}
	testParseExpr(t, s, expr)

	s = "f or e and not d = a + b * -c"
	expr = &ExprBinOp{op: OP_OR,
		left: "f", right: &ExprBinOp{op: OP_AND,
			left: "e", right: &ExprUnOp{op: OP_NOT,
				kid: &ExprBinOp{op: OP_EQ,
					left: "d", right: &ExprBinOp{op: OP_ADD,
						left: "a", right: &ExprBinOp{op: OP_MUL,
							left: "b", right: &ExprUnOp{op: OP_NEG,
								kid: "c"}}}}}}}
	testParseExpr(t, s, expr)

	s = "not not - - a"
	expr = &ExprUnOp{op: OP_NOT,
		kid: &ExprUnOp{op: OP_NOT,
			kid: &ExprUnOp{op: OP_NEG,
				kid: &ExprUnOp{op: OP_NEG,
					kid: "a"}}}}
	testParseExpr(t, s, expr)
}
