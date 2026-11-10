// Command bench generates a large JSON file and times pj, and whichever of jq,
// jaq, gojq and gron are installed, on a few typical workloads.
//
//	go run ./bench gen 500000 > /tmp/big.json
//	go run ./bench run /tmp/big.json [-svg docs/benchmark.svg]
//	go run ./bench commands
//
// It reports wall-clock time and peak memory of each command, with output sent
// to /dev/null, and can draw the comparison as an SVG chart.  Each command
// runs -runs times (3 by default) and the fastest run is reported, which
// discounts runs slowed down by whatever else the machine is doing.  It is
// deliberately simple otherwise: one file shape, no statistics.  The commands
// subcommand prints the exact command lines as the README's markdown table.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "commands" {
		fmt.Print(commandsTable())
		return
	}
	if len(os.Args) < 3 {
		usage()
	}
	switch os.Args[1] {
	case "gen":
		n, err := strconv.Atoi(os.Args[2])
		if err != nil {
			usage()
		}
		gen(n)
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		svgPath := fs.String("svg", "", "write the results as an SVG chart to this file")
		runs := fs.Int("runs", 3, "run each command this many times and keep the fastest")
		fs.Parse(os.Args[3:])
		run(os.Args[2], *svgPath, max(*runs, 1))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:\n  bench gen N > file.json                         generate a file with N items\n  bench run file.json [-svg out.svg] [-runs N]   time pj, jq, jaq, gojq and gron on it\n  bench commands                                  print the command lines as a markdown table")
	os.Exit(2)
}

// gen writes {"meta": {...}, "items": [N items]} to stdout, one compact line,
// as a machine-generated export would be.
func gen(n int) {
	rnd := rand.New(rand.NewSource(1))
	out := bufio.NewWriterSize(os.Stdout, 1<<20)
	defer out.Flush()
	out.WriteString(`{"meta": {"version": 1}, "items": [`)
	tags := []string{"a", "b", "c"}
	for i := 0; i < n; i++ {
		if i > 0 {
			out.WriteByte(',')
		}
		item := map[string]any{
			"id":      i,
			"name":    fmt.Sprintf("user%d", i),
			"email":   fmt.Sprintf("user%d@example.com", i),
			"active":  i%3 == 0,
			"price":   float64(int(rnd.Float64()*20000)) / 100,
			"tags":    tags[:i%4],
			"address": map[string]string{"city": "Boston", "zip": "02101"},
		}
		b, err := json.Marshal(item)
		if err != nil {
			panic(err)
		}
		out.Write(b)
	}
	out.WriteString("]}\n")
}

// A workload is a query in pj's syntax and the equivalent in jq's, which jaq
// and gojq share, and in gron's; nil where a tool has no equivalent.  A flat
// workload reads the JPV form of the file instead of the JSON.
type workload struct {
	name string
	pj   []string
	jq   []string
	gron []string
	flat bool
}

// jqLikeTools are jq implementations with the same command line.
var jqLikeTools = []string{"jq", "jaq", "gojq"}

// args are the arguments tool runs with for the workload on file, or nil if
// it has no equivalent.
func (w workload) args(tool, file string) []string {
	var args []string
	switch tool {
	case "pj":
		args = slices.Concat([]string{"-color", "never"}, w.pj)
	case "gron":
		args = w.gron
	default:
		args = w.jq
	}
	if args == nil {
		return nil
	}
	return slices.Concat(args, []string{w.input(tool, file)})
}

// input is the file tool reads for the workload: file itself, or for a flat
// workload, its JPV form, written by writeJPV.
func (w workload) input(tool, file string) string {
	if !w.flat {
		return file
	}
	return jpvFile(file)
}

func jpvFile(file string) string {
	return strings.TrimSuffix(file, filepath.Ext(file)) + ".jpv"
}

var workloads = []workload{
	{"pretty-print", []string{}, []string{"."}, nil, false},
	{"$.items[*].name", []string{"$.items[*].name"}, []string{".items[].name"}, nil, false},
	{"filter price > 190", []string{"-json-lines", "$.items[?@.price > 190]"}, []string{"-c", ".items[] | select(.price > 190)"}, nil, false},
	{"regex filter on email", []string{"-json-lines", `$.items[?match(@.email, "user1.*@example\\.com")]`}, []string{"-c", `.items[] | select(.email | test("^user1.*@example\\.com$"))`}, nil, false},
	{"first 10 items (file arg)", []string{"$.items[:10]"}, []string{".items[:10][]"}, nil, false},
	{"$..email (any depth)", []string{"$..email"}, []string{`.. | objects | select(has("email")) | .email`}, nil, false},
	{"JSON to lines", []string{"-out", "jpv"}, nil, []string{"--no-sort"}, false},
	// gron -u has no figure: on the 208 MB gron form of a 76 MB file, it was
	// killed after 86 seconds at 26 GB of memory.
	{"lines to JSON", []string{"-in", "jpv"}, nil, nil, true},
}

// A result is one measurement: wall time and peak memory, or ok=false if the
// tool was not run on this workload or failed.
type result struct {
	seconds float64
	mb      float64
	ok      bool
	failed  bool
}

func (r result) String() string {
	if r.failed {
		return "failed"
	}
	if !r.ok {
		return "n/a"
	}
	return fmt.Sprintf("%5.2fs %4.0fMB", r.seconds, r.mb)
}

func run(file string, svgPath string, runs int) {
	pj := buildJP()
	tools := []string{"pj"}
	paths := []string{pj}
	for _, name := range append(slices.Clone(jqLikeTools), "gron") {
		if path, err := exec.LookPath(name); err == nil {
			tools = append(tools, name)
			paths = append(paths, path)
		} else {
			fmt.Fprintf(os.Stderr, "%s not found, skipping\n", name)
		}
	}
	writeJPV(pj, file)
	// results[w][t] is the result of tool t on workload w.
	results := make([][]result, len(workloads))
	fmt.Printf("%-28s", "workload")
	for _, tool := range tools {
		fmt.Printf(" %13s", tool)
	}
	fmt.Println()
	for i, w := range workloads {
		results[i] = make([]result, len(tools))
		for t, tool := range tools {
			if args := w.args(tool, file); args != nil {
				results[i][t] = fastest(runs, paths[t], args)
			}
		}
		fmt.Printf("%-28s", w.name)
		for _, r := range results[i] {
			fmt.Printf(" %13s", r)
		}
		fmt.Println()
	}
	if svgPath != "" {
		if err := os.WriteFile(svgPath, []byte(chartSVG(tools, results, file, runs)), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "chart written to", svgPath)
	}
}

// writeJPV writes the JPV form of file, which flat workloads read.
func writeJPV(pj, file string) {
	path := jpvFile(file)
	out, err := os.Create(path)
	if err == nil {
		cmd := exec.Command(pj, "-color", "never", "-out", "jpv", file)
		cmd.Stdout = out
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "writing %s: %v\n", path, err)
		os.Exit(1)
	}
}

func buildJP() string {
	path := filepath.Join(os.TempDir(), "pj-bench")
	cmd := exec.Command("go", "build", "-o", path, "./cmd/pj")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building pj:", err)
		os.Exit(1)
	}
	return path
}

// fastest measures the command runs times and returns the fastest run.
func fastest(runs int, name string, args []string) result {
	var best result
	for range runs {
		r := measure(name, args)
		if !r.ok {
			return r
		}
		if !best.ok || r.seconds < best.seconds {
			best = r
		}
	}
	return best
}

// measure runs the command with output to /dev/null.
func measure(name string, args []string) result {
	cmd := exec.Command(name, args...)
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	defer devnull.Close()
	cmd.Stdout = devnull
	cmd.Stderr = os.Stderr
	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)
	if err != nil {
		return result{failed: true}
	}
	rss := cmd.ProcessState.SysUsage().(*syscall.Rusage).Maxrss
	if runtime.GOOS != "darwin" {
		rss *= 1024 // Linux reports KB, macOS bytes
	}
	return result{seconds: elapsed.Seconds(), mb: float64(rss) / (1 << 20), ok: true}
}

// commandsTable is a markdown table of the command line each tool runs for
// each workload, as a shell would take it, on a file called big.json.
func commandsTable() string {
	var b strings.Builder
	b.WriteString("| Workload | pj | jq, jaq, gojq | gron |\n|---|---|---|---|\n")
	for _, w := range workloads {
		fmt.Fprintf(&b, "| %s", w.name)
		for _, tool := range []string{"pj", "jq", "gron"} {
			cell := "n/a"
			if args := w.args(tool, "big.json"); args != nil {
				cell = markdownCode(tool + " " + shellJoin(args))
			}
			fmt.Fprintf(&b, " | %s", cell)
		}
		b.WriteString(" |\n")
	}
	return b.String()
}

// shellJoin joins arguments into a command line, single-quoting those that
// need it.
func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if arg != "" && strings.Trim(arg, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./") == "" {
			quoted[i] = arg
		} else {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
		}
	}
	return strings.Join(quoted, " ")
}

// markdownCode is s as a code span in a table cell, where | must be escaped.
func markdownCode(s string) string {
	return "`" + strings.ReplaceAll(s, "|", `\|`) + "`"
}
