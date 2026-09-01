package keyval

import (
	"bytes"
)

type KV struct {
	mem map[string][]byte
}

func (kv *KV) Open() error {
	kv.mem = map[string][]byte{} // empty
	return nil
}

func (kv *KV) Close() error { return nil }

// Get gets the value
func (kv *KV) Get(key []byte) ([]byte, bool, error) {
	val, ok := kv.mem[string(key)]
	return val, ok, nil
}

// Set reports whether changed
func (kv *KV) Set(key []byte, val []byte) (bool, error) {
	prev, exist := kv.mem[string(key)]
	kv.mem[string(key)] = val
	updated := !exist || !bytes.Equal(prev, val)
	return updated, nil
}

// Del reports whether changed
func (kv *KV) Del(key []byte) (bool, error) {
	_, deleted := kv.mem[string(key)]
	delete(kv.mem, string(key))
	return deleted, nil
}
