package main

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runPJ executes the pj program using 'go run' with the given args and input
func runPJ(t *testing.T, input string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	// Build the command: go run . [args...]
	cmdArgs := append([]string{"run", "."}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Stdin = strings.NewReader(input)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run pj: %v", err)
		}
	}

	return stdout, stderr, exitCode
}

// TestMainHelp_Examples tests examples from the main help text
func TestMainHelp_Examples(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{
			name:  "extract field from array items",
			input: `{"users":[{"name":"Alice"},{"name":"Bob"}]}`,
			args:  []string{"$.users[*].name"},
			want:  "\"Alice\"\n\"Bob\"\n",
		},
		{
			name:  "compact output (single line)",
			input: `{"name":"test","age":30,"address":{"city":"Boston","state":"MA"}}`,
			args:  []string{"-json-compact", "-json-compact-width", "0"},
			want:  `{"name": "test","age": 30,"address": {"city": "Boston","state": "MA"}}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", stdout, tt.want)
			}
		})
	}
}

// TestInputHelp_Examples tests examples from -help-input
func TestInputHelp_Examples(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{
			name:  "csv without header - arrays",
			input: "John,Doe,30\nJane,Smith,25\n",
			args:  []string{"-in", "csv", "-json-compact"},
			want:  "[\"John\", \"Doe\", 30]\n[\"Jane\", \"Smith\", 25]\n",
		},
		{
			name:  "csv with explicit header - objects",
			input: "John,Doe,30\nJane,Smith,25\n",
			args:  []string{"-in", "csv", "-csv-header", "name,surname,age", "-json-compact"},
			want:  `{"name": "John","surname": "Doe","age": 30}` + "\n" + `{"name": "Jane","surname": "Smith","age": 25}` + "\n",
		},
		{
			name:  "csv-with-header - objects",
			input: "name,surname,age\nJohn,Doe,30\nJane,Smith,25\n",
			args:  []string{"-in", "csvh", "-json-compact"},
			want:  `{"name": "John","surname": "Doe","age": 30}` + "\n" + `{"name": "Jane","surname": "Smith","age": 25}` + "\n",
		},
		{
			name:  "JSON Lines",
			input: `{"name":"Alice","age":30}` + "\n" + `{"name":"Bob","age":25}` + "\n",
			args:  []string{"-json-compact"},
			want:  `{"name": "Alice", "age": 30}` + "\n" + `{"name": "Bob", "age": 25}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", stdout, tt.want)
			}
		})
	}
}

// TestTransformHelp_Examples tests examples from -help-transforms
func TestTransformHelp_Examples(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{
			name:  "JSONPath - get name field",
			input: `{"name":"Alice","age":30}`,
			args:  []string{"$.name"},
			want:  "\"Alice\"\n",
		},
		{
			name:  "JSONPath - first item",
			input: `{"items":[1,2,3]}`,
			args:  []string{"$.items[0]"},
			want:  "1\n",
		},
		{
			name:  "JSONPath - last item",
			input: `{"items":[1,2,3]}`,
			args:  []string{"$.items[-1]"},
			want:  "3\n",
		},
		{
			name:  "JSONPath - slice items 2-5",
			input: `{"items":[0,1,2,3,4,5,6]}`,
			args:  []string{"-json-compact", "$.items[2:5]"},
			want:  "2\n3\n4\n",
		},
		{
			name:  "JSONPath - all items",
			input: `{"items":[1,2,3]}`,
			args:  []string{"-json-compact", "$.items[*]"},
			want:  "1\n2\n3\n",
		},
		{
			name:  "JSONPath - all names at any depth",
			input: `{"name":"Alice","child":{"name":"Bob"}}`,
			args:  []string{"$..name"},
			want:  "\"Alice\"\n\"Bob\"\n",
		},
		{
			name:  "JSONPath - filter by price",
			input: `{"items":[{"name":"apple","price":50},{"name":"banana","price":150}]}`,
			args:  []string{"$.items[?@.price < 100].name"},
			want:  "\"apple\"\n",
		},
		{
			name:  "split - array to stream",
			input: `[1,2,3]`,
			args:  []string{"-json-compact", "split"},
			want:  "1\n2\n3\n",
		},
		{
			name:  "join - stream to array",
			input: "1\n2\n3\n",
			args:  []string{"-in", "json", "-json-compact", "join"},
			want:  "[1, 2, 3]\n",
		},
		{
			name:  "depth=1 - truncate nested",
			input: `{"a":1,"b":{"c":2,"d":3}}`,
			args:  []string{"depth=1"},
			want:  "{\n  \"a\": 1,\n  \"b\": {...}\n}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", stdout, tt.want)
			}
		})
	}
}

// TestTransformHelp_CombiningExamples tests combining transforms examples
func TestTransformHelp_CombiningExamples(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{
			name:  "filter with JSONPath",
			input: `[{"name":"Alice","age":25},{"name":"Bob","age":15}]`,
			args:  []string{"-json-compact", "$[?@.age > 18]"},
			want:  `{"name": "Alice", "age": 25}` + "\n",
		},
		{
			name:  "extract nested field from all items",
			input: `{"users":[{"address":{"city":"Boston"}},{"address":{"city":"NYC"}}]}`,
			args:  []string{"$.users[*]", "split", "$.address.city"},
			want:  "\"Boston\"\n\"NYC\"\n",
		},
		{
			name:  "get all names and join to array",
			input: `{"name":"Alice","child":{"name":"Bob","child":{"name":"Charlie"}}}`,
			args:  []string{"-json-compact", "$..name", "join"},
			want:  "[\"Alice\", \"Bob\", \"Charlie\"]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", stdout, tt.want)
			}
		})
	}
}

// TestTransformHelp_CookbookExamples tests JSONPath cookbook examples
func TestTransformHelp_CookbookExamples(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{
			name:  "extract all email addresses",
			input: `{"user":{"email":"alice@example.com"},"admin":{"email":"bob@example.com"}}`,
			args:  []string{"$..email"},
			want:  "\"alice@example.com\"\n\"bob@example.com\"\n",
		},
		{
			name:  "get last 10 items from array",
			input: `{"items":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]}`,
			args:  []string{"-json-compact", "$.items[-10:]"},
			want:  "6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n",
		},
		{
			name:  "filter objects by field value",
			input: `{"users":[{"name":"Alice","active":true},{"name":"Bob","active":false}]}`,
			args:  []string{"$.users[?@.active == true].name"},
			want:  "\"Alice\"\n",
		},
		{
			name:  "get all prices and format as array",
			input: `{"items":[{"price":10},{"price":20},{"price":30}]}`,
			args:  []string{"-json-compact", "$..price", "join"},
			want:  "[10, 20, 30]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", stdout, tt.want)
			}
		})
	}
}

// TestOutputHelp_Examples tests output format examples
func TestOutputHelp_Examples(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{
			name:  "JPV output format",
			input: `{"name":"Alice","scores":[95,87]}`,
			args:  []string{"-out", "jpv"},
			want:  "$.name = \"Alice\"\n$.scores[0] = 95\n$.scores[1] = 87\n\n",
		},
		{
			name:  "compact JSON",
			input: `{"name":"test","nested":{"value":42}}`,
			args:  []string{"-json-compact"},
			want:  `{"name": "test","nested": {"value": 42}}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", stdout, tt.want)
			}
		})
	}
}

// TestCSVConversions tests CSV-specific conversions mentioned in help
func TestCSVConversions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{
			name:  "CSV empty field to null",
			input: "Alice,,30\n",
			args:  []string{"-in", "csv", "-json-compact"},
			want:  "[\"Alice\", null, 30]\n",
		},
		{
			name:  "CSV boolean conversion",
			input: "Alice,true,false\n",
			args:  []string{"-in", "csv", "-json-compact"},
			want:  "[\"Alice\", true, false]\n",
		},
		{
			name:  "CSV number parsing",
			input: "Alice,30,3.14,-5,1.5e2\n",
			args:  []string{"-in", "csv", "-json-compact"},
			want:  "[\"Alice\", 30, 3.14, -5, 1.5e2]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", stdout, tt.want)
			}
		})
	}
}

// TestDeprecatedSyntax tests that deprecated syntax produces helpful errors
func TestDeprecatedSyntax(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		args             []string
		wantErrSubstring string
	}{
		{
			name:             "deprecated .key syntax",
			input:            `{"name":"Alice"}`,
			args:             []string{".name"},
			wantErrSubstring: "Use JSONPath instead: '$.name'",
		},
		{
			name:             "deprecated ...key syntax",
			input:            `{"child":{"name":"Alice"}}`,
			args:             []string{"...name"},
			wantErrSubstring: "Use JSONPath instead: '$..name'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit code, got 0")
			}
			combined := stdout + stderr
			if !strings.Contains(combined, tt.wantErrSubstring) {
				t.Errorf("expected error to contain %q, got:\nstdout: %s\nstderr: %s",
					tt.wantErrSubstring, stdout, stderr)
			}
		})
	}
}

// TestCookbookHelp_Examples tests all examples from -help-cookbook
func TestCookbookHelp_Examples(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		// HEAD-LIKE OPERATIONS
		{
			name:  "cookbook: first 10 items",
			input: `{"items":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]}`,
			args:  []string{"-json-compact", "$.items[:10]"},
			want:  "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n",
		},
		{
			name:  "cookbook: first 5 users",
			input: `{"users":[{"id":1},{"id":2},{"id":3},{"id":4},{"id":5},{"id":6}]}`,
			args:  []string{"-json-compact", "$.users[:5]"},
			want:  `{"id": 1}` + "\n" + `{"id": 2}` + "\n" + `{"id": 3}` + "\n" + `{"id": 4}` + "\n" + `{"id": 5}` + "\n",
		},

		// TAIL-LIKE OPERATIONS
		{
			name:  "cookbook: last 10 items",
			input: `{"items":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]}`,
			args:  []string{"-json-compact", "$.items[-10:]"},
			want:  "6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n",
		},
		{
			name:  "cookbook: last 3 log entries",
			input: `{"logs":["a","b","c","d","e"]}`,
			args:  []string{"-json-compact", "$.logs[-3:]"},
			want:  "\"c\"\n\"d\"\n\"e\"\n",
		},

		// GREP-LIKE OPERATIONS
		{
			name:  "cookbook: find users with gmail in email",
			input: `{"users":[{"name":"Alice","email":"alice@gmail.com"},{"name":"Bob","email":"bob@yahoo.com"},{"name":"Charlie","email":"charlie@gmail.com"}]}`,
			args:  []string{"-json-compact", "$.users[?@.email]"},
			want:  `{"name": "Alice", "email": "alice@gmail.com"}` + "\n" + `{"name": "Bob", "email": "bob@yahoo.com"}` + "\n" + `{"name": "Charlie", "email": "charlie@gmail.com"}` + "\n",
		},
		{
			name:  "cookbook: find expensive items",
			input: `{"products":[{"name":"cheap","price":50},{"name":"expensive","price":150}]}`,
			args:  []string{"-json-compact", "$.products[?@.price > 100]"},
			want:  `{"name": "expensive", "price": 150}` + "\n",
		},
		{
			name:  "cookbook: find error logs",
			input: `{"logs":[{"level":"info","msg":"ok"},{"level":"error","msg":"bad"}]}`,
			args:  []string{"-json-compact", "$..[?@.level == \"error\"]"},
			want:  `{"level": "error", "msg": "bad"}` + "\n",
		},

		// COMBINING PATTERNS
		{
			name:  "cookbook: filter active users",
			input: `{"users":[{"name":"Alice","active":true},{"name":"Bob","active":false},{"name":"Charlie","active":true}]}`,
			args:  []string{"-json-compact", "$.users[?@.active == true]"},
			want:  `{"name": "Alice", "active": true}` + "\n" + `{"name": "Charlie", "active": true}` + "\n",
		},
		{
			name:  "cookbook: filter expensive items",
			input: `{"products":[{"price":50},{"price":150},{"price":200},{"price":75},{"price":300}]}`,
			args:  []string{"-json-compact", "$.products[?@.price > 100]"},
			want:  `{"price": 150}` + "\n" + `{"price": 200}` + "\n" + `{"price": 300}` + "\n",
		},
		{
			name:  "cookbook: extract emails to array",
			input: `{"user":{"email":"alice@example.com"},"admin":{"email":"bob@example.com"}}`,
			args:  []string{"-json-compact", "$..email", "join"},
			want:  "[\"alice@example.com\", \"bob@example.com\"]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", stdout, tt.want)
			}
		})
	}
}

// TestMainUsage_Examples tests examples from the main usage/help text
func TestMainUsage_Examples(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{
			name:  "main: extract field from array items",
			input: `{"users":[{"name":"Alice"},{"name":"Bob"}]}`,
			args:  []string{"$.users[*].name"},
			want:  "\"Alice\"\n\"Bob\"\n",
		},
		{
			name:  "main: filter and transform",
			input: `{"items":[{"name":"cheap","price":50},{"name":"expensive","price":150}]}`,
			args:  []string{"-json-compact", "$.items[?@.price < 100]", "split"},
			want:  `{"name": "cheap", "price": 50}` + "\n",
		},
		{
			name:  "main: convert array to JSON Lines",
			input: `[{"id":1},{"id":2},{"id":3}]`,
			args:  []string{"-json-lines", "split"},
			want:  `{"id": 1}` + "\n" + `{"id": 2}` + "\n" + `{"id": 3}` + "\n",
		},
		{
			name:  "main: CSV to JSON Lines",
			input: "name,age\nAlice,30\nBob,25\n",
			args:  []string{"-in", "csvh", "-json-lines"},
			want:  `{"name": "Alice", "age": 30}` + "\n" + `{"name": "Bob", "age": 25}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", stdout, tt.want)
			}
		})
	}
}

// TestJSONLinesFlag tests that -json-lines works as an alias for -json-compact
func TestJSONLinesFlag(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{
			name:  "json-lines flag",
			input: `[1,2,3]`,
			args:  []string{"-json-lines", "split"},
			want:  "1\n2\n3\n",
		},
		{
			name:  "json-compact flag (same result)",
			input: `[1,2,3]`,
			args:  []string{"-json-compact", "split"},
			want:  "1\n2\n3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("unexpected exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", stdout, tt.want)
			}
		})
	}
}

// TestReadme_LogFiltering checks the README's JSON Lines log-filtering recipe:
// a filter selects children, so the stream must be wrapped with join first.
func TestReadme_LogFiltering(t *testing.T) {
	input := "{\"level\":\"error\",\"message\":\"boom\"}\n{\"level\":\"info\",\"message\":\"ok\"}\n"
	stdout, stderr, exitCode := runPJ(t, input, "-json-compact", "join", `$[?@.level == "error"]`, "$.message")
	if exitCode != 0 {
		t.Fatalf("exit code %d, stderr: %s", exitCode, stderr)
	}
	if want := "\"boom\"\n"; stdout != want {
		t.Errorf("got %q, want %q", stdout, want)
	}
}

// TestAutoDetect_Formats checks that input format auto-detection handles the
// formats documented in the README, including the JPV round trip and top-level
// scalars.
func TestAutoDetect_Formats(t *testing.T) {
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{
			name:  "jpv input is detected (README round trip)",
			input: "$.email = \"a@x.com\"\n",
			args:  []string{"-json-compact"},
			want:  "{\"email\": \"a@x.com\"}\n",
		},
		{
			name:  "jpv input with quoted key is detected",
			input: "$[\"a b\"] = 1\n",
			args:  []string{"-json-compact"},
			want:  "{\"a b\": 1}\n",
		},
		{
			name:  "top-level number falls back to json",
			input: "42\n",
			args:  []string{"-json-compact"},
			want:  "42\n",
		},
		{
			name:  "top-level string falls back to json",
			input: "\"hello\"\n",
			args:  []string{"-json-compact"},
			want:  "\"hello\"\n",
		},
		{
			name:  "csv with header is still detected",
			input: "name,age\nAlice,30\n",
			args:  []string{"-json-compact"},
			want:  "{\"name\": \"Alice\", \"age\": 30}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got %q, want %q", stdout, tt.want)
			}
		})
	}
}

// infiniteArrayReader produces "[1,1,1,..." forever.
type infiniteArrayReader struct{ started bool }

func (r *infiniteArrayReader) Read(p []byte) (int, error) {
	n := 0
	if !r.started {
		p[0] = '['
		n = 1
		r.started = true
	}
	for ; n+1 < len(p); n += 2 {
		p[n], p[n+1] = '1', ','
	}
	return n, nil
}

// TestInputMode_StopsReading checks that a slice query exits as soon as it has
// its results on input that never ends, both when told the input is a single
// value and when left to work it out.
func TestInputMode_StopsReading(t *testing.T) {
	for _, args := range [][]string{
		{"-in", "json", "-json-compact", "$[:3]"},
		{"-json-compact", "$[:3]"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "go", append([]string{"run", "."}, args...)...)
			cmd.Stdin = &infiniteArrayReader{}
			var outBuf, errBuf bytes.Buffer
			cmd.Stdout = &outBuf
			cmd.Stderr = &errBuf
			if err := cmd.Run(); err != nil {
				t.Fatalf("pj did not exit cleanly: %v (stderr: %s)", err, errBuf.String())
			}
			if want := "1\n1\n1\n"; outBuf.String() != want {
				t.Errorf("got %q, want %q", outBuf.String(), want)
			}
		})
	}
}

// TestInputMode_Streams checks that streams of values are processed in full
// unless the input is declared to be a single value.  The query $.a is done
// with each value as soon as it finds the key, so it is where the modes differ.
func TestInputMode_Streams(t *testing.T) {
	const jsonLines = "{\"a\": 1, \"b\": 0}\n{\"a\": 2, \"b\": 0}\n"
	// The shape of pj's own pretty-printed output: not JSON Lines.
	const prettyStream = "{\n  \"a\": 1,\n  \"b\": 0\n}\n{\n  \"a\": 2,\n  \"b\": 0\n}\n"
	tests := []struct {
		name  string
		input string
		args  []string
		want  string
	}{
		{"json lines, auto", jsonLines, []string{"-json-compact", "$.a"}, "1\n2\n"},
		{"pretty stream, auto", prettyStream, []string{"-json-compact", "$.a"}, "1\n2\n"},
		{"json lines, -in json-stream", jsonLines, []string{"-in", "json-stream", "-json-compact", "$.a"}, "1\n2\n"},
		{"json lines, -in jsonl alias", jsonLines, []string{"-in", "jsonl", "-json-compact", "$.a"}, "1\n2\n"},
		{"json lines, -in json stops after the first value", jsonLines, []string{"-in", "json", "-json-compact", "$.a"}, "1\n"},
		{"query consuming each value fully still sees all values", jsonLines, []string{"-in", "json", "-json-compact", "$..a"}, "1\n2\n"},
		// join makes one value of the stream, so the slice can stop early.
		{"join then slice", jsonLines, []string{"-json-compact", "join", "$[:1].a"}, "1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, tt.input, tt.args...)
			if exitCode != 0 {
				t.Fatalf("exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got %q, want %q", stdout, tt.want)
			}
		})
	}
}

// TestInputFiles checks reading from file arguments and the input mode
// inferred from their extension.
func TestInputFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	twoValues := "{\"a\": 1, \"b\": 0}\n{\"a\": 2, \"b\": 0}\n"
	single := write("single.json", twoValues)
	stream := write("stream.jsonl", twoValues)
	other := write("other.txt", twoValues)
	one := write("one.json", "{\"a\": 1, \"b\": 0}\n")
	two := write("two.json", "{\"a\": 2, \"b\": 0}\n")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{".json is a single value", []string{"-json-compact", "$.a", single}, "1\n"},
		{".jsonl is a stream", []string{"-json-compact", "$.a", stream}, "1\n2\n"},
		{"other extension is worked out", []string{"-json-compact", "$.a", other}, "1\n2\n"},
		{"-in json-stream overrides .json", []string{"-in", "json-stream", "-json-compact", "$.a", single}, "1\n2\n"},
		{"file before transform", []string{"-json-compact", stream, "$.a"}, "1\n2\n"},
		{"several files are a stream", []string{"-json-compact", "$.a", one, two}, "1\n2\n"},
		{"-file flag", []string{"-json-compact", "-file", stream, "$.a"}, "1\n2\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, "", tt.args...)
			if exitCode != 0 {
				t.Fatalf("exit code %d, stderr: %s", exitCode, stderr)
			}
			if stdout != tt.want {
				t.Errorf("got %q, want %q", stdout, tt.want)
			}
		})
	}

	stdout, stderr, exitCode := runPJ(t, "", "$.a", filepath.Join(dir, "missing.json"))
	if exitCode == 0 || !strings.Contains(stderr, "missing.json") {
		t.Errorf("missing file: exit %d, stdout %q, stderr %q", exitCode, stdout, stderr)
	}
}

// pausingReader yields its first chunk, then blocks until released, then EOF.
type pausingReader struct {
	first   string
	release chan struct{}
	state   int
}

func (r *pausingReader) Read(p []byte) (int, error) {
	switch r.state {
	case 0:
		r.state = 1
		return copy(p, r.first), nil
	case 1:
		<-r.release
		r.state = 2
		return copy(p, "{\"a\": 2}\n"), nil
	default:
		return 0, io.EOF
	}
}

// TestOutputFlushedWhenInputStalls checks that a result reaches a pipe while
// the input is still open and paused, rather than sitting in the output buffer
// until the input ends.
func TestOutputFlushedWhenInputStalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".", "-json-compact", "$.a")
	in := &pausingReader{first: "{\"a\": 1}\n", release: make(chan struct{})}
	cmd.Stdin = in
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// The first result must come out while the input is paused.
	lines := make(chan string, 2)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	select {
	case line := <-lines:
		if line != "1" {
			t.Errorf("first line: got %q, want %q", line, "1")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no output while the input was paused: output is being held back")
	}

	close(in.release)
	if line, ok := <-lines; !ok || line != "2" {
		t.Errorf("second line: got %q (ok=%v), want %q", line, ok, "2")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("pj failed: %v (stderr: %s)", err, errBuf.String())
	}
}

// TestRawOutput checks -raw and -r: string results as plain text, other
// values as JSON, and a clear error with JPV output.
func TestRawOutput(t *testing.T) {
	input := `{"users": [{"name": "bo"}, {"name": "café"}, {"name": 3}]}`
	for _, flag := range []string{"-raw", "-r"} {
		stdout, stderr, exitCode := runPJ(t, input, flag, "$.users[*].name")
		if exitCode != 0 {
			t.Fatalf("%s: exit code %d, stderr: %s", flag, exitCode, stderr)
		}
		if want := "bo\ncafé\n3\n"; stdout != want {
			t.Errorf("%s: got %q, want %q", flag, stdout, want)
		}
	}
	_, stderr, exitCode := runPJ(t, `"x"`, "-raw", "-out", "jpv")
	if exitCode == 0 || !strings.HasSuffix(stderr, "\n") || !strings.Contains(stderr, "-raw") {
		t.Errorf("-raw with jpv: exit %d, stderr %q; want an error mentioning -raw, ending in a newline", exitCode, stderr)
	}
}

// TestInvalidInput checks that a syntax error is reported once, that the
// output gets as far as the input did, and that pj exits with status 1, with
// no stack trace, whichever stages the truncated input goes through.
func TestInvalidInput(t *testing.T) {
	// The tab inside the string is invalid JSON, in the middle of an object
	// in the middle of an array.
	const input = "{\"a\": [1, 2, {\"b\": \"x\ty\"}]}"
	tests := []struct {
		args       []string
		wantPrefix string
	}{
		{nil, "{\n  \"a\": [\n    1, 2,"},
		{[]string{"-json-compact", "$.a"}, "[1, 2,"},
		{[]string{"-json-compact", "$..a"}, "[1, 2,"},
		{[]string{"-json-compact", "$.a[*]"}, "1\n2\n"},
		{[]string{"-json-compact", "$.a[?@.b]"}, ""},
		{[]string{"-json-compact", "$.a", "split"}, "1\n2\n"},
		{[]string{"-json-compact", "$.a", "split", "join"}, "[1, 2,"},
		{[]string{"-json-compact", "$.a[*]", "join"}, "[1, 2,"},
		{[]string{"-json-compact", "join"}, "[{\"a\": [1, 2,"},
		{[]string{"-json-compact", "depth=1"}, "{\"a\": ["},
		{[]string{"-out", "jpv"}, "$.a[0] = 1\n$.a[1] = 2\n"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			stdout, stderr, exitCode := runPJ(t, input, tt.args...)
			if exitCode != 1 {
				t.Errorf("exit code %d, want 1", exitCode)
			}
			if !strings.HasPrefix(stderr, "pj: error while parsing: syntax error at L1,C22:") || strings.Contains(stderr, "goroutine") {
				t.Errorf("stderr %q, want just the syntax error", stderr)
			}
			if !strings.HasPrefix(stdout, tt.wantPrefix) {
				t.Errorf("stdout %q, want it to start with %q", stdout, tt.wantPrefix)
			}
		})
	}

	// An error between values leaves the values before it intact.
	stdout, stderr, exitCode := runPJ(t, "1 2 ]", "-json-compact")
	if exitCode != 1 || stdout != "1\n2\n" || !strings.HasPrefix(stderr, "pj: error while parsing:") {
		t.Errorf("got exit %d, stdout %q, stderr %q", exitCode, stdout, stderr)
	}
}
