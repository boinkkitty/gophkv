package main

import (
	"fmt"
	"io"
	"os"

	"github.com/boinkkitty/gophkv/internal/sql"
)

// main starts the demo binary.
func main() {
	if err := runDemo(os.Stdout, ".demo_db"); err != nil {
		fmt.Fprintf(os.Stderr, "demo failed: %v\n", err)
		os.Exit(1)
	}
}

// runDemo opens a demo database, executes a few SQL statements, and prints the results.
func runDemo(w io.Writer, file string) error {
	db := sql.DB{}
	db.KV.Log.FileName = file
	defer os.Remove(file)

	if err := db.Open(); err != nil {
		return err
	}
	defer db.Close()

	statements := []string{
		"create table link (time int64, src string, dst string, primary key (src, dst));",
		"insert into link values (123, 'bob', 'alice');",
		"select time from link where dst = 'alice' and src = 'bob';",
		"update link set time = 456 where dst = 'alice' and src = 'bob';",
		"select time from link where dst = 'alice' and src = 'bob';",
		"delete from link where src = 'bob' and dst = 'alice';",
		"select time from link where dst = 'alice' and src = 'bob';",
	}

	for _, s := range statements {
		fmt.Fprintf(w, "SQL> %s\n", s)

		p := sql.NewParser(s)
		stmt, err := p.ParseStmt()
		if err != nil {
			return err
		}

		result, err := db.ExecStmt(stmt)
		if err != nil {
			return err
		}

		fmt.Fprintf(w, "Updated: %d\n", result.Updated)
		if len(result.Header) > 0 {
			fmt.Fprintf(w, "Header: %v\n", result.Header)
		}
		if len(result.Values) > 0 {
			fmt.Fprintf(w, "Values: %v\n", result.Values)
		}
		fmt.Fprintln(w)
	}

	return nil
}
