package table

import "github.com/boinkkitty/gophkv/internal/keyval"

type DB struct {
	KV keyval.KV
}

func (db *DB) Open() error  { return db.KV.Open() }
func (db *DB) Close() error { return db.KV.Close() }

// Selects from KV
func (db *DB) Select(schema *Schema, row Row) (bool, error) {
	key := row.EncodeKey(schema)
	val, ok, err := db.KV.Get(key)
	if err != nil || !ok {
		return false, err
	}
	if err := row.DecodeVal(schema, val); err != nil {
		return false, err
	}
	return true, nil
}

// Inserts into KV
func (db *DB) Insert(schema *Schema, row Row) (updated bool, err error) {
	key := row.EncodeKey(schema)
	val := row.EncodeVal(schema)
	return db.KV.SetEx(key, val, keyval.ModeInsert)
}

// Upserts into KV
func (db *DB) Upsert(schema *Schema, row Row) (updated bool, err error) {
	key := row.EncodeKey(schema)
	val := row.EncodeVal(schema)
	return db.KV.SetEx(key, val, keyval.ModeUpsert)
}

// Updates into KV
func (db *DB) Update(schema *Schema, row Row) (updated bool, err error) {
	key := row.EncodeKey(schema)
	val := row.EncodeVal(schema)
	return db.KV.SetEx(key, val, keyval.ModeUpdate)
}

// Deletes from KV
func (db *DB) Delete(schema *Schema, row Row) (deleted bool, err error) {
	key := row.EncodeKey(schema)
	return db.KV.Del(key)
}
