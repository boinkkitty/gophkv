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
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q\n%s", want, got)
		}
	}
}
