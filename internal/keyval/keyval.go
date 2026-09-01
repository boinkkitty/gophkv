package keyval

import (
	"bytes"
)

type KV struct {
	mem map[string][]byte
	Log Log
}

// Open opens the log and rebuilds the in-memory index from it.
func (kv *KV) Open() error {
	if err := kv.Log.Open(); err != nil {
		return err
	}
	kv.mem = map[string][]byte{} // empty
	for {
		entry := Entry{}
		eof, err := kv.Log.Read(&entry)
		if err != nil {
			return err
		} else if eof {
			break
		}

		if entry.deleted {
			delete(kv.mem, string(entry.key))
		} else {
			kv.mem[string(entry.key)] = entry.val
		}
	}
	return nil
}

// Close closes the backing log file.
func (kv *KV) Close() error { return kv.Log.Close() }

// Get returns the value stored for key, if present.
func (kv *KV) Get(key []byte) ([]byte, bool, error) {
	val, ok := kv.mem[string(key)]
	return val, ok, nil
}

type UpdateMode int

const (
	ModeUpsert UpdateMode = 0 // insert or update
	ModeInsert UpdateMode = 1 // insert new
	ModeUpdate UpdateMode = 2 // update existing
)

// SetEx stores val for key using the requested update mode.
func (kv *KV) SetEx(key []byte, val []byte, mode UpdateMode) (bool, error) {
	prev, exist := kv.mem[string(key)]

	var updated bool
	var err error

	switch mode {
	case ModeUpsert:
		updated = !exist || !bytes.Equal(prev, val)
	case ModeInsert:
		updated = !exist
	case ModeUpdate:
		updated = exist && !bytes.Equal(prev, val)
	default:
		panic("unreachable")
	}
	if updated {
		if err = kv.Log.Write(&Entry{key: key, val: val}); err != nil {
			return false, err
		}
		kv.mem[string(key)] = val
	}
	return updated, err
}

// Set stores val for key and reports whether the value changed.
func (kv *KV) Set(key []byte, val []byte) (bool, error) {
	return kv.SetEx(key, val, ModeUpsert)
}

// Del removes key and reports whether anything was deleted.
func (kv *KV) Del(key []byte) (bool, error) {
	_, deleted := kv.mem[string(key)]
	if deleted {
		if err := kv.Log.Write(&Entry{
			key:     key,
			deleted: true,
		}); err != nil {
			return false, err
		}
		delete(kv.mem, string(key))
	}
	return deleted, nil
}
