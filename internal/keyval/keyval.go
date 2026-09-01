package keyval

import (
	"bytes"
)

type KV struct {
	mem map[string][]byte
	log Log
}

// Open opens the log and rebuilds the in-memory index from it.
func (kv *KV) Open() error {
	if err := kv.log.Open(); err != nil {
		return err
	}
	kv.mem = map[string][]byte{} // empty
	for {
		entry := Entry{}
		eof, err := kv.log.Read(&entry)
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
func (kv *KV) Close() error { return kv.log.Close() }

// Get returns the value stored for key, if present.
func (kv *KV) Get(key []byte) ([]byte, bool, error) {
	val, ok := kv.mem[string(key)]
	return val, ok, nil
}

// Set stores val for key and reports whether the value changed.
func (kv *KV) Set(key []byte, val []byte) (bool, error) {
	prev, exist := kv.mem[string(key)]
	updated := !exist || !bytes.Equal(prev, val)
	if updated {
		if err := kv.log.Write(&Entry{
			key:     key,
			val:     val,
			deleted: false,
		}); err != nil {
			return false, err
		}
		kv.mem[string(key)] = val
	}
	return updated, nil
}

// Del removes key and reports whether anything was deleted.
func (kv *KV) Del(key []byte) (bool, error) {
	_, deleted := kv.mem[string(key)]
	if deleted {
		if err := kv.log.Write(&Entry{
			key:     key,
			deleted: true,
		}); err != nil {
			return false, err
		}
		delete(kv.mem, string(key))
	}
	return deleted, nil
}
