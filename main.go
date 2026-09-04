package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/boinkkitty/gophkv/internal/sql"
)

// main starts either the interactive SQL terminal or the scripted demo.
func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "--demo" {
		err = runDemo(os.Stdout, ".demo_db")
	} else {
		err = runTerminal(os.Stdin, os.Stdout, ".demo_db")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "demo failed: %v\n", err)
		os.Exit(1)
	}
}

// runTerminal opens a database and executes semicolon-terminated SQL from r.
func runTerminal(r io.Reader, w io.Writer, file string) error {
	db := sql.DB{}
	db.KV.Log.FileName = file
	defer os.Remove(file)

	if err := db.Open(); err != nil {
		return err
	}
	defer db.Close()

	reader := bufio.NewReader(r)
	var stmt strings.Builder
	for {
		fmt.Fprint(w, "SQL> ")
		line, err := reader.ReadString('\n')
		stmt.WriteString(line)
		if strings.Contains(line, ";") {
			sqlText := strings.TrimSpace(stmt.String())
			stmt.Reset()
			if sqlText == "" {
				if err == io.EOF {
					return nil
				}
				continue
			}

			p := sql.NewParser(sqlText)
			parsed, parseErr := p.ParseStmt()
			if parseErr != nil {
				fmt.Fprintf(w, "Error: %v\n", parseErr)
			} else {
				result, execErr := db.ExecStmt(parsed)
				if execErr != nil {
					fmt.Fprintf(w, "Error: %v\n", execErr)
				} else {
					fmt.Fprintf(w, "Updated: %d\n", result.Updated)
					if len(result.Header) > 0 {
						fmt.Fprintf(w, "Header: %v\n", result.Header)
					}
					if len(result.Values) > 0 {
						fmt.Fprintf(w, "Values: %v\n", result.Values)
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
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

	schema, err := db.GetSchema("link")
	if err != nil {
		return err
	}
	for _, row := range []sql.Row{
		{
			{Type: sql.TypeI64, I64: 200},
			{Type: sql.TypeStr, Str: []byte("bob")},
			{Type: sql.TypeStr, Str: []byte("zed")},
		},
		{
			{Type: sql.TypeI64, I64: 300},
			{Type: sql.TypeStr, Str: []byte("carol")},
			{Type: sql.TypeStr, Str: []byte("yan")},
		},
		{
			{Type: sql.TypeI64, I64: 400},
			{Type: sql.TypeStr, Str: []byte("dave")},
			{Type: sql.TypeStr, Str: []byte("xena")},
		},
	} {
		if _, err := db.Insert(&schema, row); err != nil {
			return err
		}
	}

	fmt.Fprintln(w, "Range [bob, dave]:")
	iter, err := db.Range(&schema, &sql.RangeReq{
		StartCmp: sql.OP_GE,
		StopCmp:  sql.OP_LE,
		Start:    []sql.Cell{{Type: sql.TypeStr, Str: []byte("bob")}},
		Stop:     []sql.Cell{{Type: sql.TypeStr, Str: []byte("dave")}},
	})
	if err != nil {
		return err
	}
	for ; iter.Valid(); err = iter.Next() {
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "Range row: %v\n", iter.Row())
	}
	if err != nil {
		return err
	}

	return nil
}
