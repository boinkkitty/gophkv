package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunDemoPrintsDBFlow(t *testing.T) {
	var out bytes.Buffer

	if err := runDemo(&out, ".test_main_db"); err != nil {
		t.Fatalf("runDemo returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"SQL> create table",
		"SQL> insert into link",
		"Updated: 1",
		"Header: [time]",
		"Values:",
		"Range [bob, dave]:",
		"Range row:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q\n%s", want, got)
		}
	}
}

func TestRunTerminalExecutesStatements(t *testing.T) {
	var in bytes.Buffer
	var out bytes.Buffer

	in.WriteString("create table t (id int64, name string, primary key (id));\n")
	in.WriteString("insert into t values (1, 'alice');\n")
	in.WriteString("select name from t where id = 1;\n")

	if err := runTerminal(&in, &out, ".test_terminal_db"); err != nil {
		t.Fatalf("runTerminal returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"SQL> ",
		"Updated: 0",
		"Updated: 1",
		"Header: [name]",
		"Values: [[{2 0 [97 108 105 99 101]}]]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q\n%s", want, got)
		}
	}
}
