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
	iter   *keyval.KVIterator
	valid  bool // decode result (err != ErrOutOfRange)
	row    Row  // decode result
}

// decodeKVIter decodes KVITerator into row primary columns.
func decodeKVIter(schema *Schema, iter *keyval.KVIterator, row Row) (bool, error) {
	// Check iter valid
	if !iter.Valid() {
		return false, nil
	}
	// Decode Key
	if err := row.DecodeKey(schema, iter.Key()); err == ErrOutOfRange {
		return false, nil // OOR
	} else if err != nil {
		return false, err
	}
	// Decode Val
	if err := row.DecodeVal(schema, iter.Val()); err != nil {
		return false, err
	}
	return true, nil
}

// Is iteration finished?
func (iter *RowIterator) Valid() bool {
	return iter.valid
}

// Row checks iterator current row.
func (iter *RowIterator) Row() Row {
	utils.Check(iter.valid)
	return iter.row
}

// Next moves iterator forward to next row.
func (iter *RowIterator) Next() (err error) {
	if err = iter.iter.Next(); err != nil {
		return err
	}
	iter.valid, err = decodeKVIter(iter.schema, iter.iter, iter.row)
	return err
}

// Seek returns the first position >= primary key.
func (db *DB) Seek(schema *Schema, row Row) (*RowIterator, error) {
	iter, err := db.KV.Seek(row.EncodeKey(schema))
	if err != nil {
		return nil, err
	}
	valid, err := decodeKVIter(schema, iter, row)
	if err != nil {
		return nil, err
	}
	return &RowIterator{
		schema,
		iter,
		valid,
		row,
	}, nil
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
		r.Header = ptr.Cols
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

// makePKey builds a primary-key row from parsed key expressions.
func makePKey(schema *Schema, pkey []NamedCell) (Row, error) {
	if len(schema.PKey) != len(pkey) {
		return nil, errors.New("not primary key")
	}
	row := schema.NewRow()
	for _, idx1 := range schema.PKey {
		col := schema.Cols[idx1]
		idx2 := slices.IndexFunc(pkey, func(expr NamedCell) bool {
			return expr.Column == col.Name && expr.Value.Type == col.Type
		})
		if idx2 < 0 {
			return nil, errors.New("not primary key")
		}
		row[idx1] = pkey[idx2].Value
	}
	return row, nil
}

// subsetRow returns a row containing only the requested column indexes.
func subsetRow(row Row, indices []int) (out Row) {
	for _, idx := range indices {
		out = append(out, row[idx])
	}
	return
}

// execSelect executes a parsed SELECT statement.
func (db *DB) execSelect(stmt *StmtSelect) ([]Row, error) {
	schema, err := db.GetSchema(stmt.Table)
	if err != nil {
		return nil, err
	}
	indices, err := lookupColumns(schema.Cols, stmt.Cols)
	if err != nil {
		return nil, err
	}

	row, err := makePKey(&schema, stmt.Keys)
	if err != nil {
		return nil, err
	}
	if ok, err := db.Select(&schema, row); err != nil || !ok {
		return nil, err
	}

	row = subsetRow(row, indices)
	return []Row{row}, nil
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

// execUpdate executes a parsed UPDATE statement.
func (db *DB) execUpdate(stmt *StmtUpdate) (count int, err error) {
	schema, err := db.GetSchema(stmt.Table)
	if err != nil {
		return 0, err
	}

	row, err := makePKey(&schema, stmt.Keys)
	if err != nil {
		return 0, err
	}
	if ok, err := db.Select(&schema, row); err != nil || !ok {
		return 0, err
	}

	if err = fillNonPKey(&schema, stmt.Value, row); err != nil {
		return 0, err
	}
	updated, err := db.Update(&schema, row)
	if err != nil {
		return 0, err
	}
	if updated {
		count++
	}
	return count, nil
}

// execDelete executes a parsed DELETE statement.
func (db *DB) execDelete(stmt *StmtDelete) (count int, err error) {
	schema, err := db.GetSchema(stmt.Table)
	if err != nil {
		return 0, err
	}

	row, err := makePKey(&schema, stmt.Keys)
	if err != nil {
		return 0, err
	}

	updated, err := db.Delete(&schema, row)
	if err != nil {
		return 0, err
	}
	if updated {
		count++
	}
	return count, nil
}
