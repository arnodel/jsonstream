package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// runJP executes the jp program using 'go run' with the given args and input
func runJP(t *testing.T, input string, args ...string) (stdout, stderr string, exitCode int) {
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
			t.Fatalf("failed to run jp: %v", err)
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
			stdout, stderr, exitCode := runJP(t, tt.input, tt.args...)
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
			stdout, stderr, exitCode := runJP(t, tt.input, tt.args...)
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
			stdout, stderr, exitCode := runJP(t, tt.input, tt.args...)
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
			stdout, stderr, exitCode := runJP(t, tt.input, tt.args...)
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
			stdout, stderr, exitCode := runJP(t, tt.input, tt.args...)
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
			stdout, stderr, exitCode := runJP(t, tt.input, tt.args...)
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
			stdout, stderr, exitCode := runJP(t, tt.input, tt.args...)
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
			stdout, stderr, exitCode := runJP(t, tt.input, tt.args...)
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
