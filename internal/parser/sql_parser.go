package parser

import (
	"errors"
	"strconv"
	"strings"

	"github.com/boinkkitty/gophkv/internal/table"
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
	table string
	cols  []string
	keys  []NamedCell
}

type NamedCell struct {
	column string
	value  table.Cell
}

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

// tryKeyword consumes kw if it appears at the current position.
func (p *Parser) tryKeyword(kw string) bool {
	p.skipSpaces()
	if !(p.pos+len(kw) <= len(p.buf) && strings.EqualFold(p.buf[p.pos:p.pos+len(kw)], kw)) {
		return false
	}
	if p.pos+len(kw) < len(p.buf) && !isSeparator(p.buf[p.pos+len(kw)]) {
		return false
	}
	p.pos += len(kw)
	return true
}

// tryPunctuation consumes token if it appears at the current position.
func (p *Parser) tryPunctuation(token string) bool {
	p.skipSpaces()
	if !(p.pos+len(token) <= len(p.buf) && p.buf[p.pos:p.pos+len(token)] == token) {
		return false
	}
	p.pos += len(token)
	return true
}

// tryName parses an identifier from the current position.
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
func (p *Parser) parseValue(out *table.Cell) error {
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
func (p *Parser) parseString(out *table.Cell) error {
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
			out.Type = table.TypeStr
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
func (p *Parser) parseInt(out *table.Cell) (err error) {
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
	out.Type = table.TypeI64
	p.pos = cur
	return nil
}

// parseEqual parses a column equality expression.
func (p *Parser) parseEqual(out *NamedCell) error {
	var ok bool
	out.column, ok = p.tryName()
	if !ok {
		return errors.New("expect column")
	}
	if !p.tryPunctuation("=") {
		return errors.New("expect =")
	}
	return p.parseValue(&out.value)
}

// parseSelect parses a SELECT statement into out.
func (p *Parser) parseSelect(out *StmtSelect) error {
	if !p.tryKeyword("SELECT") {
		return errors.New("expect keyword")
	}
	for !p.tryKeyword("FROM") {
		if len(out.cols) > 0 && !p.tryPunctuation(",") {
			return errors.New("expect comma")
		}
		if name, ok := p.tryName(); ok {
			out.cols = append(out.cols, name)
		} else {
			return errors.New("expect column")
		}
	}
	if len(out.cols) == 0 {
		return errors.New("expect column list")
	}
	var ok bool
	if out.table, ok = p.tryName(); !ok {
		return errors.New("expect table name")
	}
	return p.parseWhere(&out.keys)
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

// isEnd reports whether the parser has reached the end of input.
func (p *Parser) isEnd() bool {
	p.skipSpaces()
	return p.pos >= len(p.buf)
}
