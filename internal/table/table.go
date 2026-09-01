package table

import "github.com/boinkkitty/gophkv/internal/keyval"

type DB struct {
	KV keyval.KV
}

// Open opens the backing key-value store.
func (db *DB) Open() error { return db.KV.Open() }

// Close closes the backing key-value store.
func (db *DB) Close() error { return db.KV.Close() }

// Select loads a row by its primary key.
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
