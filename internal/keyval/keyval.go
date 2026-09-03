package keyval

import (
	"bytes"
	"slices"

	"github.com/boinkkitty/gophkv/internal/utils"
)

type KV struct {
	Log  Log
	keys [][]byte
	vals [][]byte
}

// Open opens the log and rebuilds the in-memory index from it.
func (kv *KV) Open() error {
	if err := kv.Log.Open(); err != nil {
		return err
	}

	entries := []Entry{}
	for {
		ent := Entry{}
		eof, err := kv.Log.Read(&ent)
		if err != nil {
			return err
		} else if eof {
			break
		}
		entries = append(entries, ent)
	}

	// sort keys
	slices.SortStableFunc(entries, func(a, b Entry) int {
		return bytes.Compare(a.key, b.key)
	})
	kv.keys, kv.vals = kv.keys[:0], kv.vals[:0]

	for _, ent := range entries {
		n := len(kv.keys)
		if n > 0 && bytes.Equal(kv.keys[n-1], ent.key) {
			kv.keys, kv.vals = kv.keys[:n-1], kv.vals[:n-1]
		}
		if !ent.deleted {
			kv.keys = append(kv.keys, ent.key)
			kv.vals = append(kv.vals, ent.val)
		}
	}

	return nil
}

// Close closes the backing log file.
func (kv *KV) Close() error { return kv.Log.Close() }

// Get returns the value stored for key, if present.
func (kv *KV) Get(key []byte) ([]byte, bool, error) {
	if idx, ok := slices.BinarySearchFunc(kv.keys, key, bytes.Compare); ok {
		return kv.vals[idx], true, nil
	}
	return nil, false, nil
}

type UpdateMode int

const (
	ModeUpsert UpdateMode = 0 // insert or update
	ModeInsert UpdateMode = 1 // insert new
	ModeUpdate UpdateMode = 2 // update existing
)

// SetEx stores val for key using the requested update mode.
func (kv *KV) SetEx(key []byte, val []byte, mode UpdateMode) (bool, error) {
	idx, exist := slices.BinarySearchFunc(kv.keys, key, bytes.Compare)

	var updated bool
	var err error

	switch mode {
	case ModeUpsert:
		updated = !exist || !bytes.Equal(kv.vals[idx], val)
	case ModeInsert:
		updated = !exist
	case ModeUpdate:
		updated = exist && !bytes.Equal(kv.vals[idx], val)
	default:
		panic("unreachable")
	}
	if updated {
		if err = kv.Log.Write(&Entry{key: key, val: val}); err != nil {
			return false, err
		}
		if exist {
			kv.vals[idx] = val
		} else {
			kv.keys = slices.Insert(kv.keys, idx, key)
			kv.vals = slices.Insert(kv.vals, idx, val)
		}
	}
	return updated, err
}

// Set stores val for key and reports whether the value changed.
func (kv *KV) Set(key []byte, val []byte) (bool, error) {
	return kv.SetEx(key, val, ModeUpsert)
}

// Del removes key and reports whether anything was deleted.
func (kv *KV) Del(key []byte) (bool, error) {
	if idx, ok := slices.BinarySearchFunc(kv.keys, key, bytes.Compare); ok {
		if err := kv.Log.Write(&Entry{key: key, deleted: true}); err != nil {
			return false, err
		}
		kv.keys = slices.Delete(kv.keys, idx, idx+1)
		kv.vals = slices.Delete(kv.vals, idx, idx+1)
		return true, nil
	}
	return false, nil
}

type KVIterator struct {
	keys [][]byte
	vals [][]byte
	pos  int
}

// Seek returns an iterator positioned at the first key >= key.
func (kv *KV) Seek(key []byte) (*KVIterator, error) {
	pos, _ := slices.BinarySearchFunc(kv.keys, key, bytes.Compare)
	return &KVIterator{
		keys: kv.keys,
		vals: kv.vals,
		pos:  pos,
	}, nil
}

// Valid reports whether the iterator points at an existing entry.
func (iter *KVIterator) Valid() bool {
	return 0 <= iter.pos && iter.pos < len(iter.keys)
}

// Key returns the iterator's current key.
func (iter *KVIterator) Key() []byte {
	utils.Check(iter.Valid())
	return iter.keys[iter.pos]
}

// Val returns the iterator's current value.
func (iter *KVIterator) Val() []byte {
	utils.Check(iter.Valid())
	return iter.vals[iter.pos]
}

// Next advances the iterator by one position.
func (iter *KVIterator) Next() error {
	if iter.pos < len(iter.keys) {
		iter.pos++
	}
	return nil
}

// Prev moves the iterator back by one position.
func (iter *KVIterator) Prev() error {
	if iter.pos >= 0 {
		iter.pos--
	}
	return nil
}

// RangedKVIter wraps a KVIterator with inclusive start/stop bounds.
// desc == false: query start <= key && key <= stop
// desc == true: query start >= key && key >= stop
type RangedKVIter struct {
	iter KVIterator
	stop []byte
	desc bool // flag for desc and asc order by
}

// Key returns the current key within the bounded range.
func (iter *RangedKVIter) Key() []byte {
	return iter.iter.Key()
}

// Val returns the current value within the bounded range.
func (iter *RangedKVIter) Val() []byte {
	return iter.iter.Val()
}

// Valid reports whether the iterator is still within the requested bounds.
func (iter *RangedKVIter) Valid() bool {
	if !iter.iter.Valid() {
		return false
	}
	r := bytes.Compare(iter.iter.Key(), iter.stop)
	if iter.desc && r < 0 {
		return false
	} else if !iter.desc && r > 0 {
		return false
	}
	return true
}

// Next advances in the configured scan direction until the range is exhausted.
func (iter *RangedKVIter) Next() error {
	if !iter.Valid() {
		return nil
	}
	if iter.desc {
		return iter.iter.Prev()
	} else {
		return iter.iter.Next()
	}
}

// Range returns an iterator that starts at start and stops once stop is crossed.
// In ascending mode it yields keys in [start, stop]. In descending mode it yields
// keys in [stop, start], starting from the greatest key <= start.
func (kv *KV) Range(start, stop []byte, desc bool) (*RangedKVIter, error) {
	iter, err := kv.Seek(start)
	if err != nil {
		return nil, err
	}
	if desc && (!iter.Valid() || bytes.Compare(iter.Key(), start) > 0) {
		if err = iter.Prev(); err != nil {
			return nil, err
		}
	}
	return &RangedKVIter{
		iter: *iter,
		stop: stop,
		desc: desc,
	}, nil
}
