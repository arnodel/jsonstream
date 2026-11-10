package main

import (
	"os"
	"strings"
	"testing"
)

// The README lists the exact commands the benchmark runs: it must be updated
// when a workload changes.
func TestReadmeCommands(t *testing.T) {
	readme, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range suites {
		if want := commandsTable(s); !strings.Contains(string(readme), want) {
			t.Errorf("README.md does not contain the commands of bench %s; replace its table with the matching output of go run ./bench commands:\n%s", s.command, want)
		}
	}
}
