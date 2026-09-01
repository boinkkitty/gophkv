package keyval

import (
	"io"
	"os"
)

type Log struct {
	FileName string
	fp       *os.File
}

// Open opens or creates the log file.
func (log *Log) Open() (err error) {
	log.fp, err = createFileSync(log.FileName)
	return err
}

// Close closes the log file.
func (log *Log) Close() error {
	return log.fp.Close()
}

// Write appends an entry to the log and flushes it to disk.
func (log *Log) Write(ent *Entry) error {
	if _, err := log.fp.Write(ent.Encode()); err != nil {
		return err
	}
	return log.fp.Sync() //fsync
}

// Read decodes the next log entry and treats torn tail records as EOF.
func (log *Log) Read(ent *Entry) (eof bool, err error) {
	err = ent.Decode(log.fp)
	if err == io.EOF || err == io.ErrUnexpectedEOF || err == ErrBadSum {
		return true, nil
	} else if err != nil {
		return false, err
	} else {
		return false, nil
	}
}
