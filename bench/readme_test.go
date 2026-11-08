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
	if want := commandsTable(); !strings.Contains(string(readme), want) {
		t.Errorf("README.md does not contain the benchmark commands; replace its table with the output of go run ./bench commands:\n%s", want)
	}
}
