package sql

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/boinkkitty/gophkv/internal/keyval"
	"github.com/boinkkitty/gophkv/internal/utils"
)

type DB struct {
	KV     keyval.KV
	tables map[string]Schema
}

// Open opens the backing key-value store.
func (db *DB) Open() error {
	db.tables = map[string]Schema{}
	return db.KV.Open()
}

// Close closes the backing key-value store.
func (db *DB) Close() error { return db.KV.Close() }

// Select loads a row by its primary key.
func (db *DB) Select(schema *Schema, row Row) (ok bool, err error) {
	key := row.EncodeKey(schema)
	val, ok, err := db.KV.Get(key)
	if err != nil || !ok {
		return ok, err
	}
	if err = row.DecodeVal(schema, val); err != nil {
		return false, err
	}
	return true, nil
}

// Insert stores a new row when the key does not already exist.
func (db *DB) Insert(schema *Schema, row Row) (updated bool, err error) {
	key := row.EncodeKey(schema)
	val := row.EncodeVal(schema)
	return db.KV.SetEx(key, val, keyval.ModeInsert)
}

// Upsert stores a row whether or not the key already exists.
func (db *DB) Upsert(schema *Schema, row Row) (updated bool, err error) {
	key := row.EncodeKey(schema)
	val := row.EncodeVal(schema)
	return db.KV.SetEx(key, val, keyval.ModeUpsert)
}

// Update stores a row only when the key already exists.
func (db *DB) Update(schema *Schema, row Row) (updated bool, err error) {
	key := row.EncodeKey(schema)
	val := row.EncodeVal(schema)
	return db.KV.SetEx(key, val, keyval.ModeUpdate)
}

// Delete removes a row by its primary key.
func (db *DB) Delete(schema *Schema, row Row) (deleted bool, err error) {
	key := row.EncodeKey(schema)
	return db.KV.Del(key)
}

type RowIterator struct {
	schema *Schema
	iter   *keyval.RangedKVIter
	valid  bool
	row    Row
}

// decodeKVIter decodes the current key/value entry into row.
func decodeKVIter(schema *Schema, iter *keyval.RangedKVIter, row Row) (bool, error) {
	if !iter.Valid() {
		return false, nil
	}
	if err := row.DecodeKey(schema, iter.Key()); err != nil {
		utils.Check(err != ErrOutOfRange)
		return false, err
	}
	if err := row.DecodeVal(schema, iter.Val()); err != nil {
		return false, err
	}
	return true, nil
}

// Valid reports whether the iterator is positioned on a row.
func (iter *RowIterator) Valid() bool {
	return iter.valid
}

// Row returns the current decoded row.
func (iter *RowIterator) Row() Row {
	utils.Check(iter.valid)
	return iter.row
}

// Next advances the iterator to the next row in scan order.
func (iter *RowIterator) Next() (err error) {
	if err = iter.iter.Next(); err != nil {
		return err
	}
	iter.valid, err = decodeKVIter(iter.schema, iter.iter, iter.row)
	return err
}

// Seek returns the first row whose primary key is >= the provided key prefix.
func (db *DB) Seek(schema *Schema, row Row) (*RowIterator, error) {
	start := make([]Cell, len(schema.PKey))
	for i, idx := range schema.PKey {
		utils.Check(row[idx].Type == schema.Cols[idx].Type)
		start[i] = row[idx]
	}
	return db.Range(schema, &RangeReq{
		StartCmp: OP_GE,
		StopCmp:  OP_LE,
		Start:    start,
		Stop:     nil,
	})
}

// RangeReq describes a bounded primary-key scan.
type RangeReq struct {
	StartCmp ExprOp
	StopCmp  ExprOp
	Start    []Cell
	Stop     []Cell
}

// suffixPositive reports whether a comparison boundary should be encoded with
// the 0xFF positive suffix so the bound falls just above the raw prefix.
func suffixPositive(op ExprOp) bool {
	switch op {
	case OP_LE, OP_GT:
		return true
	case OP_GE, OP_LT:
		return false
	default:
		panic("unreachable")
	}
}

// isDescending reports whether a range using op should scan keys in reverse.
func isDescending(op ExprOp) bool {
	switch op {
	case OP_LE, OP_LT:
		return true
	case OP_GE, OP_GT:
		return false
	default:
		panic("unreachable")
	}
}

// Range returns an iterator over the requested primary-key interval.
func (db *DB) Range(schema *Schema, req *RangeReq) (*RowIterator, error) {
	utils.Check(isDescending(req.StartCmp) != isDescending(req.StopCmp))
	start := EncodeKeyPrefix(schema, req.Start, suffixPositive(req.StartCmp))
	stop := EncodeKeyPrefix(schema, req.Stop, suffixPositive(req.StopCmp))
	desc := isDescending(req.StartCmp)
	iter, err := db.KV.Range(start, stop, desc)
	if err != nil {
		return nil, err
	}
	row := schema.NewRow()
	valid, err := decodeKVIter(schema, iter, row)
	if err != nil {
		return nil, err
	}
	return &RowIterator{schema, iter, valid, row}, nil
}

type SQLResult struct {
	Updated int
	Header  []string
	Values  []Row
}

// ExecStmt executes one parsed SQL statement against the database.
func (db *DB) ExecStmt(stmt interface{}) (r SQLResult, err error) {
	switch ptr := stmt.(type) {
	case *StmtCreatTable:
		err = db.execCreateTable(ptr)
	case *StmtSelect:
		r.Header = exprs2header(ptr.Cols)
		r.Values, err = db.execSelect(ptr)
	case *StmtInsert:
		r.Updated, err = db.execInsert(ptr)
	case *StmtUpdate:
		r.Updated, err = db.execUpdate(ptr)
	case *StmtDelete:
		r.Updated, err = db.execDelete(ptr)
	default:
		panic("unreachable")
	}
	return
}

// execCreateTable creates and persists one schema from a parsed statement.
func (db *DB) execCreateTable(stmt *StmtCreatTable) (err error) {
	if _, err := db.GetSchema(stmt.Table); err == nil {
		return errors.New("duplicate table name")
	}

	schema := Schema{
		Table: stmt.Table,
		Cols:  stmt.Cols,
	}
	if schema.PKey, err = lookupColumns(stmt.Cols, stmt.PKey); err != nil {
		return err
	}

	val, err := json.Marshal(schema)
	utils.Check(err == nil)
	if _, err = db.KV.Set([]byte("@schema_"+stmt.Table), val); err != nil {
		return err
	}

	db.tables[schema.Table] = schema
	return nil
}

// GetSchema loads a schema by table name, using the cache when possible.
func (db *DB) GetSchema(table string) (Schema, error) {
	schema, ok := db.tables[table]
	if !ok {
		val, ok, err := db.KV.Get([]byte("@schema_" + table))
		if err == nil && ok {
			err = json.Unmarshal(val, &schema)
		}
		if err != nil {
			return Schema{}, err
		}
		if !ok {
			return Schema{}, errors.New("table is not found")
		}
		db.tables[table] = schema
	}
	return schema, nil
}

// lookupColumns maps column names to their indexes within cols.
func lookupColumns(cols []Column, names []string) (indices []int, err error) {
	for _, name := range names {
		idx := slices.IndexFunc(cols, func(col Column) bool {
			return col.Name == name
		})
		if idx < 0 {
			return nil, errors.New("column is not found")
		}
		indices = append(indices, idx)
	}
	return
}

// extractPKey converts equality matches into a primary-key cell prefix.
func extractPKey(schema *Schema, pkey []NamedCell) ([]Cell, bool) {
	if len(schema.PKey) != len(pkey) {
		return nil, false
	}
	cells := make([]Cell, 0, len(schema.PKey))
	for _, idx := range schema.PKey {
		col := schema.Cols[idx]
		match := slices.IndexFunc(pkey, func(expr NamedCell) bool {
			return expr.Column == col.Name && expr.Value.Type == col.Type
		})
		if match < 0 {
			return nil, false
		}
		cells = append(cells, pkey[match].Value)
	}
	return cells, true
}

// matchAllEq flattens an AND tree of equality expressions into name/value pairs.
func matchAllEq(cond interface{}, out []NamedCell) ([]NamedCell, bool) {
	binop, ok := cond.(*ExprBinOp)
	if ok && binop.op == OP_AND {
		if out, ok = matchAllEq(binop.left, out); !ok {
			return nil, false
		}
		if out, ok = matchAllEq(binop.right, out); !ok {
			return nil, false
		}
		return out, true
	}
	if ok && binop.op == OP_EQ {
		left, right := binop.left, binop.right
		name, ok := left.(string)
		if !ok {
			left, right = right, left
			name, ok = left.(string)
		}
		if !ok {
			return nil, false
		}
		cell, ok := right.(*Cell)
		if !ok {
			return nil, false
		}
		return append(out, NamedCell{Column: name, Value: *cell}), true
	}
	return nil, false
}

// asNameList converts either a single column name or a tuple of names into a slice.
func asNameList(expr interface{}) ([]string, bool) {
	switch e := expr.(type) {
	case string:
		return []string{e}, true
	case *ExprTuple:
		out := make([]string, 0, len(e.Kids))
		for _, kid := range e.Kids {
			name, ok := kid.(string)
			if !ok {
				return nil, false
			}
			out = append(out, name)
		}
		return out, true
	default:
		return nil, false
	}
}

// asCellList converts either a single literal cell or a tuple of cells into a slice.
func asCellList(expr interface{}) ([]Cell, bool) {
	switch e := expr.(type) {
	case *Cell:
		return []Cell{*e}, true
	case *ExprTuple:
		out := make([]Cell, 0, len(e.Kids))
		for _, kid := range e.Kids {
			cell, ok := kid.(*Cell)
			if !ok {
				return nil, false
			}
			out = append(out, *cell)
		}
		return out, true
	default:
		return nil, false
	}
}

// matchCmp extracts a comparison operator and normalized operands from an AST node.
func matchCmp(cond interface{}) (ExprOp, []string, []Cell, bool) {
	binop, ok := cond.(*ExprBinOp)
	if !ok {
		return 0, nil, nil, false
	}
	switch binop.op {
	case OP_LE, OP_GE, OP_LT, OP_GT:
	default:
		return 0, nil, nil, false
	}

	op := binop.op
	left, right := binop.left, binop.right
	names, ok := asNameList(left)
	if !ok {
		left, right = right, left
		names, ok = asNameList(left)
		switch op {
		case OP_LE:
			op = OP_GE
		case OP_GE:
			op = OP_LE
		case OP_LT:
			op = OP_GT
		case OP_GT:
			op = OP_LT
		}
	}
	if !ok {
		return 0, nil, nil, false
	}
	cells, ok := asCellList(right)
	if !ok {
		return 0, nil, nil, false
	}
	return op, names, cells, true
}

// isPKeyPrefix reports whether cols/cells match the table primary-key prefix.
func isPKeyPrefix(schema *Schema, cols []string, cells []Cell) bool {
	if len(cols) != len(cells) || len(cols) > len(schema.PKey) {
		return false
	}
	for i := range cols {
		col := schema.Cols[schema.PKey[i]]
		if col.Name != cols[i] || col.Type != cells[i].Type {
			return false
		}
	}
	return true
}

// matchRange converts inequality predicates on the primary-key prefix into a range scan.
func matchRange(schema *Schema, cond interface{}) (*RangeReq, bool) {
	binop, ok := cond.(*ExprBinOp)
	if ok && binop.op == OP_AND {
		op1, cols1, cells1, ok := matchCmp(binop.left)
		if !ok || !isPKeyPrefix(schema, cols1, cells1) {
			return nil, false
		}
		op2, cols2, cells2, ok := matchCmp(binop.right)
		if !ok || !isPKeyPrefix(schema, cols2, cells2) {
			return nil, false
		}
		if isDescending(op1) == isDescending(op2) {
			return nil, false
		}
		if isDescending(op1) {
			op1, op2 = op2, op1
			cells1, cells2 = cells2, cells1
		}
		return &RangeReq{
			StartCmp: op1,
			StopCmp:  op2,
			Start:    cells1,
			Stop:     cells2,
		}, true
	}

	op, cols, cells, ok := matchCmp(cond)
	if !ok || !isPKeyPrefix(schema, cols, cells) {
		return nil, false
	}
	stopCmp := OP_LE
	if isDescending(op) {
		stopCmp = OP_GE
	}
	return &RangeReq{
		StartCmp: op,
		StopCmp:  stopCmp,
		Start:    cells,
		Stop:     nil,
	}, true
}

// makeRange turns a supported WHERE expression into a primary-key range request.
func makeRange(schema *Schema, cond interface{}) (*RangeReq, error) {
	if keys, ok := matchAllEq(cond, nil); ok {
		if pkey, ok := extractPKey(schema, keys); ok {
			return &RangeReq{
				StartCmp: OP_GE,
				StopCmp:  OP_LE,
				Start:    pkey,
				Stop:     pkey,
			}, nil
		}
	}
	if req, ok := matchRange(schema, cond); ok {
		return req, nil
	}
	return nil, errors.New("unimplemented WHERE")
}

// execCond resolves a WHERE expression into the row iterator used by statement execution.
func (db *DB) execCond(schema *Schema, cond interface{}) (*RowIterator, error) {
	req, err := makeRange(schema, cond)
	if err != nil {
		return nil, err
	}
	return db.Range(schema, req)
}

// execSelect executes a parsed SELECT statement over a primary-key range.
func (db *DB) execSelect(stmt *StmtSelect) ([]Row, error) {
	schema, err := db.GetSchema(stmt.Table)
	if err != nil {
		return nil, err
	}

	iter, err := db.execCond(&schema, stmt.Cond)
	if err != nil {
		return nil, err
	}

	output := []Row{}
	for ; err == nil && iter.Valid(); err = iter.Next() {
		row := iter.Row()
		computed := make(Row, len(stmt.Cols))
		for i, expr := range stmt.Cols {
			cell, err := evalExpr(&schema, row, expr)
			if err != nil {
				return nil, err
			}
			computed[i] = *cell
		}
		output = append(output, computed)
	}
	if err != nil {
		return nil, err
	}
	return output, nil
}

// execInsert executes a parsed INSERT statement.
func (db *DB) execInsert(stmt *StmtInsert) (count int, err error) {
	schema, err := db.GetSchema(stmt.Table)
	if err != nil {
		return 0, err
	}
	if len(schema.Cols) != len(stmt.Value) {
		return 0, errors.New("schema mismatch")
	}
	for i := range schema.Cols {
		if schema.Cols[i].Type != stmt.Value[i].Type {
			return 0, errors.New("schema mismatch")
		}
	}

	updated, err := db.Insert(&schema, stmt.Value)
	if err != nil {
		return 0, err
	}
	if updated {
		count++
	}
	return count, nil
}

// fillNonPKey applies non-primary-key updates onto out.
func fillNonPKey(schema *Schema, updates []NamedCell, out Row) error {
	for _, expr := range updates {
		idx := slices.IndexFunc(schema.Cols, func(col Column) bool {
			return col.Name == expr.Column && col.Type == expr.Value.Type
		})
		if idx < 0 || slices.Contains(schema.PKey, idx) {
			return errors.New("cannot update column")
		}
		out[idx] = expr.Value
	}
	return nil
}

// execUpdate executes a parsed UPDATE statement over a primary-key range.
func (db *DB) execUpdate(stmt *StmtUpdate) (count int, err error) {
	schema, err := db.GetSchema(stmt.Table)
	if err != nil {
		return 0, err
	}

	iter, err := db.execCond(&schema, stmt.Cond)
	if err != nil {
		return 0, err
	}

	for ; err == nil && iter.Valid(); err = iter.Next() {
		row := iter.Row()
		updates := make([]NamedCell, len(stmt.Value))
		for i, assign := range stmt.Value {
			cell, err := evalExpr(&schema, row, assign.expr)
			if err != nil {
				return 0, err
			}
			updates[i] = NamedCell{Column: assign.column, Value: *cell}
		}

		if err = fillNonPKey(&schema, updates, row); err != nil {
			return 0, err
		}
		updated, err := db.Update(&schema, row)
		if err != nil {
			return 0, err
		}
		if updated {
			count++
		}
	}
	if err != nil {
		return 0, err
	}
	return count, nil
}

// execDelete executes a parsed DELETE statement over a primary-key range.
func (db *DB) execDelete(stmt *StmtDelete) (count int, err error) {
	schema, err := db.GetSchema(stmt.Table)
	if err != nil {
		return 0, err
	}

	iter, err := db.execCond(&schema, stmt.Cond)
	if err != nil {
		return 0, err
	}

	for ; err == nil && iter.Valid(); err = iter.Next() {
		row := iter.Row()
		updated, err := db.Delete(&schema, row)
		if err != nil {
			return 0, err
		}
		if updated {
			count++
		}
	}
	if err != nil {
		return 0, err
	}
	return count, nil
}
