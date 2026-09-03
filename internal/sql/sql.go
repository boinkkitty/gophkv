package sql

import (
	"errors"
	"strconv"
	"strings"
)

type Parser struct {
	buf string
	pos int
}

// NewParser returns a parser initialized with the input string.
func NewParser(s string) Parser {
	return Parser{buf: s, pos: 0}
}

type StmtSelect struct {
	Table string
	Cols  []string
	Keys  []NamedCell
}

type NamedCell struct {
	Column string
	Value  Cell
}

type StmtCreatTable struct {
	Table string
	Cols  []Column
	PKey  []string
}

type StmtInsert struct {
	Table string
	Value []Cell
}

type StmtUpdate struct {
	Table string
	Keys  []NamedCell
	Value []NamedCell
}

type StmtDelete struct {
	Table string
	Keys  []NamedCell
}

type ExprOp uint8

const (
	OP_ADD ExprOp = 1  // +
	OP_SUB ExprOp = 2  // -
	OP_LE  ExprOp = 12 // <=
	OP_GE  ExprOp = 13 // >=
	OP_LT  ExprOp = 14 // <
	OP_GT  ExprOp = 15 // >
)

// isSpace reports whether ch is an ASCII whitespace character.
func isSpace(ch byte) bool {
	switch ch {
	case '\t', '\n', '\v', '\f', '\r', ' ':
		return true
	}
	return false
}

// isAlpha reports whether ch is an ASCII letter.
func isAlpha(ch byte) bool {
	return 'a' <= (ch|32) && (ch|32) <= 'z'
}

// isDigit reports whether ch is an ASCII digit.
func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

// isNameStart reports whether ch can begin an identifier.
func isNameStart(ch byte) bool {
	return isAlpha(ch) || ch == '_'
}

// isNameContinue reports whether ch can continue an identifier.
func isNameContinue(ch byte) bool {
	return isAlpha(ch) || isDigit(ch) || ch == '_'
}

// isSeparator reports whether ch terminates an identifier.
func isSeparator(ch byte) bool {
	return ch < 128 && !isNameContinue(ch)
}

// skipSpaces advances past any leading whitespace.
func (p *Parser) skipSpaces() {
	for p.pos < len(p.buf) && isSpace(p.buf[p.pos]) {
		p.pos += 1
	}
}

// tryKeyword consumes the given keywords if they appear in sequence.
// Example: SELECT, CREATE TABLE, or DELETE FROM.
func (p *Parser) tryKeyword(kws ...string) bool {
	save := p.pos
	for _, kw := range kws {
		p.skipSpaces()
		if !(p.pos+len(kw) <= len(p.buf) && strings.EqualFold(p.buf[p.pos:p.pos+len(kw)], kw)) {
			p.pos = save
			return false
		}
		if p.pos+len(kw) < len(p.buf) && !isSeparator(p.buf[p.pos+len(kw)]) {
			p.pos = save
			return false
		}
		p.pos += len(kw)
	}
	return true
}

// tryPunctuation consumes token if it appears at the current position.
// Example: ,, (, ), ;, or =.
func (p *Parser) tryPunctuation(token string) bool {
	p.skipSpaces()
	if !(p.pos+len(token) <= len(p.buf) && p.buf[p.pos:p.pos+len(token)] == token) {
		return false
	}
	p.pos += len(token)
	return true
}

// tryName parses an identifier from the current position.
// Example: a, b_02, or users.
func (p *Parser) tryName() (string, bool) {
	p.skipSpaces()
	start, cur := p.pos, p.pos
	if !(cur < len(p.buf) && isNameStart(p.buf[cur])) {
		return "", false
	}
	cur++
	for cur < len(p.buf) && isNameContinue(p.buf[cur]) {
		cur++
	}
	p.pos = cur
	return p.buf[start:cur], true
}

// parseValue parses either a string or integer cell value.
func (p *Parser) parseValue(out *Cell) error {
	p.skipSpaces()
	if p.pos >= len(p.buf) {
		return errors.New("expect value")
	}
	ch := p.buf[p.pos]
	if ch == '"' || ch == '\'' {
		return p.parseString(out)
	} else if isDigit(ch) || ch == '-' || ch == '+' {
		return p.parseInt(out)
	} else {
		return errors.New("expect value")
	}
}

// parseString parses a quoted string cell value.
func (p *Parser) parseString(out *Cell) error {
	quote := p.buf[p.pos]
	cur := p.pos + 1
	for cur < len(p.buf) {
		ch := p.buf[cur]
		if ch == '\\' {
			cur++
			if cur < len(p.buf) && (p.buf[cur] == '"' || p.buf[cur] == '\'') {
				out.Str = append(out.Str, p.buf[cur])
				cur++
			} else {
				return errors.New("bad escape")
			}
		} else if ch == quote {
			out.Type = TypeStr
			p.pos = cur + 1
			return nil
		} else {
			out.Str = append(out.Str, p.buf[cur])
			cur++
		}
	}
	return errors.New("string is not terminated")
}

// parseInt parses a signed integer cell value.
func (p *Parser) parseInt(out *Cell) (err error) {
	start, cur := p.pos, p.pos
	if p.buf[cur] == '-' || p.buf[cur] == '+' {
		cur++
	}
	for cur < len(p.buf) && isDigit(p.buf[cur]) {
		cur++
	}

	if out.I64, err = strconv.ParseInt(p.buf[start:cur], 10, 64); err != nil {
		return err
	}
	out.Type = TypeI64
	p.pos = cur
	return nil
}

// parseEqual parses a column equality expression.
func (p *Parser) parseEqual(out *NamedCell) error {
	var ok bool
	out.Column, ok = p.tryName()
	if !ok {
		return errors.New("expect column")
	}
	if !p.tryPunctuation("=") {
		return errors.New("expect =")
	}
	return p.parseValue(&out.Value)
}

// parseSelect parses a SELECT statement into out.
func (p *Parser) parseSelect(out *StmtSelect) error {
	for !p.tryKeyword("FROM") {
		if len(out.Cols) > 0 && !p.tryPunctuation(",") {
			return errors.New("expect comma")
		}
		if name, ok := p.tryName(); ok {
			out.Cols = append(out.Cols, name)
		} else {
			return errors.New("expect column")
		}
	}
	if len(out.Cols) == 0 {
		return errors.New("expect column list")
	}
	var ok bool
	if out.Table, ok = p.tryName(); !ok {
		return errors.New("expect table name")
	}
	return p.parseWhere(&out.Keys)
}

// parseWhere parses a WHERE clause joined by AND.
func (p *Parser) parseWhere(out *[]NamedCell) error {
	if !p.tryKeyword("WHERE") {
		return errors.New("expect keyword")
	}
	for !p.tryPunctuation(";") {
		expr := NamedCell{}
		if len(*out) > 0 && !p.tryKeyword("AND") {
			return errors.New("expect AND")
		}
		if err := p.parseEqual(&expr); err != nil {
			return err
		}
		*out = append(*out, expr)
	}
	if len(*out) == 0 {
		return errors.New("expect where clause")
	}
	return nil
}

// parseCommaList parses a parenthesized comma-separated list.
func (p *Parser) parseCommaList(item func() error) error {
	if !p.tryPunctuation("(") {
		return errors.New("expect (")
	}
	comma := false
	for !p.tryPunctuation(")") {
		if comma && !p.tryPunctuation(",") {
			return errors.New("expect ,")
		}
		comma = true
		if err := item(); err != nil {
			return err
		}
	}
	return nil
}

// parseNameItem parses one identifier into out.
func (p *Parser) parseNameItem(out *[]string) error {
	name, ok := p.tryName()
	if !ok {
		return errors.New("expect name")
	}
	*out = append(*out, name)
	return nil
}

// parseCreateTableItem parses either a column definition or primary key clause.
func (p *Parser) parseCreateTableItem(out *StmtCreatTable) error {
	if p.tryKeyword("PRIMARY", "KEY") {
		return p.parseCommaList(func() error { return p.parseNameItem(&out.PKey) })
	}

	var ok bool
	col := Column{}
	if col.Name, ok = p.tryName(); !ok {
		return errors.New("expect name")
	}
	kind, ok := p.tryName()
	if !ok {
		return errors.New("expect name")
	}
	switch kind {
	case "int64":
		col.Type = TypeI64
	case "string":
		col.Type = TypeStr
	default:
		return errors.New("unknown column type")
	}
	out.Cols = append(out.Cols, col)
	return nil
}

// parseCreateTable parses a CREATE TABLE statement into out.
func (p *Parser) parseCreateTable(out *StmtCreatTable) error {
	var ok bool
	if out.Table, ok = p.tryName(); !ok {
		return errors.New("expect table name")
	}
	if err := p.parseCommaList(func() error { return p.parseCreateTableItem(out) }); err != nil {
		return err
	}
	if !p.tryPunctuation(";") {
		return errors.New("expect ;")
	}
	return nil
}

// parseValueItem parses one cell value into out.
func (p *Parser) parseValueItem(out *[]Cell) error {
	cell := Cell{}
	if err := p.parseValue(&cell); err != nil {
		return err
	}
	*out = append(*out, cell)
	return nil
}

// parseInsert parses an INSERT statement into out.
func (p *Parser) parseInsert(out *StmtInsert) error {
	var ok bool
	if out.Table, ok = p.tryName(); !ok {
		return errors.New("expect table name")
	}
	if !p.tryKeyword("VALUES") {
		return errors.New("expect VALUES")
	}
	if err := p.parseCommaList(func() error { return p.parseValueItem(&out.Value) }); err != nil {
		return err
	}
	if !p.tryPunctuation(";") {
		return errors.New("expect ;")
	}
	return nil
}

// parseUpdate parses an UPDATE statement into out.
func (p *Parser) parseUpdate(out *StmtUpdate) error {
	var ok bool
	if out.Table, ok = p.tryName(); !ok {
		return errors.New("expect table name")
	}
	if !p.tryKeyword("SET") {
		return errors.New("expect SET")
	}
	for !p.tryKeyword("WHERE") {
		expr := NamedCell{}
		if len(out.Value) > 0 && !p.tryKeyword(",") {
			return errors.New("expect ,")
		}
		if err := p.parseEqual(&expr); err != nil {
			return err
		}
		out.Value = append(out.Value, expr)
	}
	if len(out.Value) == 0 {
		return errors.New("expect assignment list")
	}
	p.pos -= len("WHERE")
	return p.parseWhere(&out.Keys)
}

// parseDelete parses a DELETE statement into out.
func (p *Parser) parseDelete(out *StmtDelete) error {
	var ok bool
	if out.Table, ok = p.tryName(); !ok {
		return errors.New("expect table name")
	}
	return p.parseWhere(&out.Keys)
}

// parseStmt parses one SQL statement and returns its typed representation.
func (p *Parser) parseStmt() (out interface{}, err error) {
	if p.tryKeyword("SELECT") {
		stmt := &StmtSelect{}
		err = p.parseSelect(stmt)
		out = stmt
	} else if p.tryKeyword("CREATE", "TABLE") {
		stmt := &StmtCreatTable{}
		err = p.parseCreateTable(stmt)
		out = stmt
	} else if p.tryKeyword("INSERT", "INTO") {
		stmt := &StmtInsert{}
		err = p.parseInsert(stmt)
		out = stmt
	} else if p.tryKeyword("UPDATE") {
		stmt := &StmtUpdate{}
		err = p.parseUpdate(stmt)
		out = stmt
	} else if p.tryKeyword("DELETE", "FROM") {
		stmt := &StmtDelete{}
		err = p.parseDelete(stmt)
		out = stmt
	} else {
		err = errors.New("unknown statement")
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ParseStmt parses one SQL statement and returns its typed representation.
func (p *Parser) ParseStmt() (interface{}, error) {
	return p.parseStmt()
}

// isEnd reports whether the parser has reached the end of input.
func (p *Parser) isEnd() bool {
	p.skipSpaces()
	return p.pos >= len(p.buf)
}

type ExprBinOp struct {
	op    ExprOp
	left  interface{}
	right interface{}
}

// parseAtom parses a single expression atom as either an identifier or literal cell.
func (p *Parser) parseAtom() (interface{}, error) {
	if name, ok := p.tryName(); ok {
		return name, nil
	}
	cell := &Cell{}
	if err := p.parseValue(cell); err != nil {
		return nil, err
	}
	return cell, nil
}

// parseAdd parses a left-associative chain of + and - operations over atoms.
func (p *Parser) parseAdd() (interface{}, error) {
	left, err := p.parseAtom()
	if err != nil {
		return nil, err
	}

	tokens := []string{"+", "-"}
	ops := []ExprOp{OP_ADD, OP_SUB}

	for ok := true; ok; {
		ok = false
		for i := range tokens {
			if !p.tryPunctuation(tokens[i]) {
				continue
			}

			ok = true
			right, err := p.parseAtom()
			if err != nil {
				return nil, err
			}
			left = &ExprBinOp{op: ops[i], left: left, right: right}
			break
		}
	}

	return left, nil
}
