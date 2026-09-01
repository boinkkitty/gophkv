package keyval

import (
	"bytes"

	"github.com/boinkkitty/gophkv/log"
)

type KV struct {
	mem map[string][]byte
	log log.Log
}

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
		} else if eof != nil {
			break
		}

		if entry.deleted {
			delete(kv.mem, string(entry.key))
		} else {
			kv.mem[string(entry.key)] = entry.val
		}
		kv.Set(entry.key, entry.val)
	}

	for {
		ent := Entry{}
		eof, err := kv.log.Read(&ent)
		if err != nil {
			return err
		} else if eof {
			break
		}

		if ent.deleted {
			delete(kv.mem, string(ent.key))
		} else {
			kv.mem[string(ent.key)] = ent.val
		}
	}
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
