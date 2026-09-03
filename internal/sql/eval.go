package sql

import (
	"bytes"
	"cmp"
	"errors"
	"slices"
	"strconv"
)

// evalExpr evaluates an expression AST against a row using a tree-walk interpreter.
func evalExpr(schema *Schema, row Row, expr interface{}) (*Cell, error) {
	switch e := expr.(type) {
	case string:
		idx := slices.IndexFunc(schema.Cols, func(col Column) bool {
			return col.Name == e
		})
		if idx < 0 {
			return nil, errors.New("unknown column")
		}
		return &row[idx], nil
	case *Cell:
		return e, nil
	case *ExprUnOp:
		kid, err := evalExpr(schema, row, e.kid)
		if err != nil {
			return nil, err
		}
		if e.op == OP_NEG && kid.Type == TypeI64 {
			return &Cell{Type: TypeI64, I64: -kid.I64}, nil
		}
		if e.op == OP_NOT && kid.Type == TypeI64 {
			out := int64(0)
			if kid.I64 == 0 {
				out = 1
			}
			return &Cell{Type: TypeI64, I64: out}, nil
		}
		return nil, errors.New("bad unary op")
	case *ExprBinOp:
		left, err := evalExpr(schema, row, e.left)
		if err != nil {
			return nil, err
		}
		right, err := evalExpr(schema, row, e.right)
		if err != nil {
			return nil, err
		}
		if left.Type != right.Type {
			return nil, errors.New("binary op type mismatch")
		}

		out := &Cell{Type: left.Type}
		switch e.op {
		case OP_EQ, OP_NE, OP_LE, OP_GE, OP_LT, OP_GT:
			compare := 0
			switch out.Type {
			case TypeI64:
				compare = cmp.Compare(left.I64, right.I64)
			case TypeStr:
				compare = bytes.Compare(left.Str, right.Str)
			default:
				panic("unreachable")
			}

			match := false
			switch e.op {
			case OP_EQ:
				match = compare == 0
			case OP_NE:
				match = compare != 0
			case OP_LE:
				match = compare <= 0
			case OP_GE:
				match = compare >= 0
			case OP_LT:
				match = compare < 0
			case OP_GT:
				match = compare > 0
			}
			if match {
				out.I64 = 1
			}
			return out, nil
		}

		switch {
		case e.op == OP_ADD && out.Type == TypeStr:
			out.Str = slices.Concat(left.Str, right.Str)
		case e.op == OP_ADD && out.Type == TypeI64:
			out.I64 = left.I64 + right.I64
		case e.op == OP_SUB && out.Type == TypeI64:
			out.I64 = left.I64 - right.I64
		case e.op == OP_MUL && out.Type == TypeI64:
			out.I64 = left.I64 * right.I64
		case e.op == OP_DIV && out.Type == TypeI64:
			if right.I64 == 0 {
				return nil, errors.New("division by 0")
			}
			out.I64 = left.I64 / right.I64
		case e.op == OP_AND && out.Type == TypeI64:
			if left.I64 != 0 && right.I64 != 0 {
				out.I64 = 1
			}
		case e.op == OP_OR && out.Type == TypeI64:
			if left.I64 != 0 || right.I64 != 0 {
				out.I64 = 1
			}
		default:
			return nil, errors.New("bad binary op")
		}
		return out, nil
	default:
		panic("unreachable")
	}
}

// cell2str renders one cell value into its SQL-style expression string.
func cell2str(cell *Cell) string {
	switch cell.Type {
	case TypeI64:
		return strconv.FormatInt(cell.I64, 10)
	case TypeStr:
		return string(cell.Str)
	default:
		panic("unreachable")
	}
}

// exprop2str renders one expression operator into its SQL token spelling.
func exprop2str(op ExprOp) string {
	switch op {
	case OP_ADD:
		return "+"
	case OP_SUB:
		return "-"
	case OP_MUL:
		return "*"
	case OP_DIV:
		return "/"
	case OP_EQ:
		return "="
	case OP_NE:
		return "!="
	case OP_LE:
		return "<="
	case OP_GE:
		return ">="
	case OP_LT:
		return "<"
	case OP_GT:
		return ">"
	case OP_AND:
		return "AND"
	case OP_OR:
		return "OR"
	case OP_NOT:
		return "NOT"
	case OP_NEG:
		return "-"
	default:
		panic("unreachable")
	}
}

// expr2str renders one parsed expression into a stable SQL-style string.
func expr2str(expr interface{}) string {
	switch e := expr.(type) {
	case string:
		return e
	case *Cell:
		return cell2str(e)
	case *ExprUnOp:
		switch e.op {
		case OP_NEG:
			return "-" + expr2str(e.kid)
		case OP_NOT:
			return "NOT " + expr2str(e.kid)
		default:
			panic("unreachable")
		}
	case *ExprBinOp:
		return "(" + expr2str(e.left) + " " + exprop2str(e.op) + " " + expr2str(e.right) + ")"
	default:
		panic("unreachable")
	}
}

// exprs2header renders a SELECT expression list into result header strings.
func exprs2header(cols []interface{}) []string {
	header := make([]string, 0, len(cols))
	for _, expr := range cols {
		header = append(header, expr2str(expr))
	}
	return header
}
