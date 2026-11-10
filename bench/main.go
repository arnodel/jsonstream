// Command bench generates a JSON file and times pj against other tools on it:
// whichever of jq, jaq and gojq are installed on a few typical queries, and
// gron on turning JSON into lines and back.
//
//	go run ./bench gen 500000 > /tmp/big.json
//	go run ./bench run /tmp/big.json [-svg docs/benchmark.svg]
//	go run ./bench gen 5000 > /tmp/small.json
//	go run ./bench jpv /tmp/small.json [-svg docs/benchmark-jpv.svg]
//	go run ./bench scale run|jpv [-svg docs/benchmark-scale.svg]
//	go run ./bench commands
//
// It reports wall-clock time and peak memory of each command, with output sent
// to /dev/null, and can draw the comparison as an SVG chart.  Each command
// runs -runs times (3 by default) and the fastest run is reported, which
// discounts runs slowed down by whatever else the machine is doing.  It is
// deliberately simple otherwise: one file shape, no statistics.  The commands
// subcommand prints the exact command lines as the README's markdown tables.
// The scale subcommand runs a suite on files of increasing size that it
// generates, and draws how each tool's time and memory grow.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
		for i, s := range suites {
			if i > 0 {
				fmt.Println()
			}
			fmt.Print(commandsTable(s))
		}
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
		gen(os.Stdout, n)
	case "scale":
		i := slices.IndexFunc(suites, func(s suite) bool { return s.command == os.Args[2] })
		if i < 0 {
			usage()
		}
		fs := flag.NewFlagSet("scale", flag.ExitOnError)
		svgPath := fs.String("svg", "", "write the results as an SVG chart to this file")
		runs := fs.Int("runs", 3, "run each command this many times and keep the fastest")
		fs.Parse(os.Args[3:])
		scale(suites[i], *svgPath, max(*runs, 1))
	default:
		i := slices.IndexFunc(suites, func(s suite) bool { return s.command == os.Args[1] })
		if i < 0 {
			usage()
		}
		fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
		svgPath := fs.String("svg", "", "write the results as an SVG chart to this file")
		runs := fs.Int("runs", 3, "run each command this many times and keep the fastest")
		fs.Parse(os.Args[3:])
		run(suites[i], os.Args[2], *svgPath, max(*runs, 1))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:\n  bench gen N > file.json                         generate a file with N items\n  bench run file.json [-svg out.svg] [-runs N]   time pj, jq, jaq and gojq on queries\n  bench jpv file.json [-svg out.svg] [-runs N]   time pj and gron on JSON to lines and back\n  bench scale run|jpv [-svg out.svg] [-runs N]   run a suite on files of increasing size\n  bench commands                                  print the command lines as markdown tables")
	os.Exit(2)
}

// gen writes {"meta": {...}, "items": [N items]} to w, one compact line,
// as a machine-generated export would be.
func gen(w io.Writer, n int) {
	rnd := rand.New(rand.NewSource(1))
	out := bufio.NewWriterSize(w, 1<<20)
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

// A suite compares pj with other tools that share a command line, on a set of
// workloads, run by the bench subcommand of the same name.
type suite struct {
	command   string
	others    []string
	workloads []workload
	// scaleItems are the numbers of items in the files the scale subcommand
	// runs the suite on.
	scaleItems []int
}

// A workload is a command in pj's syntax and the equivalent in the other
// tools', nil if they have none.  A flat workload reads each tool's line-based
// form of the file instead of the JSON, written by writeFlat.
type workload struct {
	name  string
	pj    []string
	other []string
	flat  bool
}

// args are the arguments tool runs with for the workload on file, or nil if
// it has no equivalent.
func (w workload) args(tool, file string) []string {
	args := w.other
	if tool == "pj" {
		args = slices.Concat([]string{"-color", "never"}, w.pj)
	}
	if args == nil {
		return nil
	}
	if w.flat {
		file = flatFile(tool, file)
	}
	return slices.Concat(args, []string{file})
}

// flatArgs are the arguments that make a tool write its line-based form of a
// JSON file.
var flatArgs = map[string][]string{
	"pj":   {"-color", "never", "-out", "jpv"},
	"gron": {"--no-sort"},
}

// flatFile is where tool's line-based form of file is written.
func flatFile(tool, file string) string {
	ext := ".jpv"
	if tool != "pj" {
		ext = "." + tool
	}
	return strings.TrimSuffix(file, filepath.Ext(file)) + ext
}

var suites = []suite{
	{"run", []string{"jq", "jaq", "gojq"}, []workload{
		{"pretty-print", []string{}, []string{"."}, false},
		{"$.items[*].name", []string{"$.items[*].name"}, []string{".items[].name"}, false},
		{"filter price > 190", []string{"-json-lines", "$.items[?@.price > 190]"}, []string{"-c", ".items[] | select(.price > 190)"}, false},
		{"regex filter on email", []string{"-json-lines", `$.items[?match(@.email, "user1.*@example\\.com")]`}, []string{"-c", `.items[] | select(.email | test("^user1.*@example\\.com$"))`}, false},
		{"first 10 items (file arg)", []string{"$.items[:10]"}, []string{".items[:10][]"}, false},
		{"$..email (any depth)", []string{"$..email"}, []string{`.. | objects | select(has("email")) | .email`}, false},
	}, []int{31250, 62500, 125000, 250000, 500000, 1000000}},
	// gron -u's memory grows much faster than its input (4.4 GB for a 0.7 MB
	// file, 23 GB for 3 MB), so this suite runs on a small file.
	{"jpv", []string{"gron"}, []workload{
		{"JSON to lines", []string{"-out", "jpv"}, []string{"--no-sort"}, false},
		{"lines to JSON", []string{"-in", "jpv"}, []string{"-u"}, true},
	}, []int{313, 625, 1250, 2500, 5000}},
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

func run(s suite, file string, svgPath string, runs int) {
	tools, paths := s.tools()
	results := s.measure(tools, paths, file, runs)
	printResults(tools, s.names(), results)
	if svgPath != "" {
		writeChart(svgPath, chartSVG(tools, s.names(), results, file, runs))
	}
}

// tools are pj, built from this checkout, and whichever of the suite's other
// tools are installed, with their paths.
func (s suite) tools() (tools, paths []string) {
	tools = []string{"pj"}
	paths = []string{buildJP()}
	for _, name := range s.others {
		if path, err := exec.LookPath(name); err == nil {
			tools = append(tools, name)
			paths = append(paths, path)
		} else {
			fmt.Fprintf(os.Stderr, "%s not found, skipping\n", name)
		}
	}
	return tools, paths
}

func (s suite) names() []string {
	names := make([]string, len(s.workloads))
	for i, w := range s.workloads {
		names[i] = w.name
	}
	return names
}

// measure runs each tool on each workload on file: results[w][t] is the
// result of tool t on workload w.
func (s suite) measure(tools, paths []string, file string, runs int) [][]result {
	if slices.ContainsFunc(s.workloads, func(w workload) bool { return w.flat }) {
		writeFlat(tools, paths, file)
	}
	results := make([][]result, len(s.workloads))
	for i, w := range s.workloads {
		results[i] = make([]result, len(tools))
		for t, tool := range tools {
			if args := w.args(tool, file); args != nil {
				results[i][t] = fastest(runs, paths[t], args)
			}
		}
	}
	return results
}

func printResults(tools, names []string, results [][]result) {
	fmt.Printf("%-28s", "workload")
	for _, tool := range tools {
		fmt.Printf(" %13s", tool)
	}
	fmt.Println()
	for i, name := range names {
		fmt.Printf("%-28s", name)
		for _, r := range results[i] {
			fmt.Printf(" %13s", r)
		}
		fmt.Println()
	}
}

func writeChart(path, svg string) {
	if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "chart written to", path)
}

// writeFlat writes each tool's line-based form of file, which flat workloads
// read.
func writeFlat(tools, paths []string, file string) {
	for t, tool := range tools {
		path := flatFile(tool, file)
		out, err := os.Create(path)
		if err == nil {
			cmd := exec.Command(paths[t], slices.Concat(flatArgs[tool], []string{file})...)
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

// commandsTable is a markdown table of the command line each tool of the
// suite runs for each workload, as a shell would take it, on a file called
// big.json.
func commandsTable(s suite) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| Workload | pj | %s |\n|---|---|---|\n", strings.Join(s.others, ", "))
	for _, w := range s.workloads {
		fmt.Fprintf(&b, "| %s", w.name)
		for _, tool := range []string{"pj", s.others[0]} {
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
