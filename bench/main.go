// Command bench generates a large JSON file and times pj, and whichever of jq,
// jaq and gojq are installed, on a few typical workloads.
//
//	go run ./bench gen 500000 > /tmp/big.json
//	go run ./bench run /tmp/big.json [-svg docs/benchmark.svg]
//
// It reports wall-clock time and peak memory of each command, with output sent
// to /dev/null, and can draw the comparison as an SVG chart.  It is
// deliberately simple: one file shape, one run per workload, no statistics.
// Run it a few times and look at the trend.
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
	"strconv"
	"syscall"
	"time"
)

func main() {
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
		fs.Parse(os.Args[3:])
		run(os.Args[2], *svgPath)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:\n  bench gen N > file.json              generate a file with N items\n  bench run file.json [-svg out.svg]   time pj, jq, jaq and gojq on it")
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
// and gojq share.
type workload struct {
	name string
	pj   []string
	jq   []string
}

// jqLikeTools are jq implementations with the same command line.
var jqLikeTools = []string{"jq", "jaq", "gojq"}

var workloads = []workload{
	{"pretty-print", []string{}, []string{"."}},
	{"JPV output", []string{"-out", "jpv"}, nil},
	{"$.items[*].name", []string{"$.items[*].name"}, []string{".items[].name"}},
	{"filter price > 190", []string{"-json-lines", "$.items[?@.price > 190]"}, []string{"-c", ".items[] | select(.price > 190)"}},
	{"regex filter on email", []string{"-json-lines", `$.items[?match(@.email, "user1.*@example\\.com")]`}, []string{"-c", `.items[] | select(.email | test("^user1.*@example\\.com$"))`}},
	{"first 10 items (file arg)", []string{"$.items[:10]"}, []string{".items[:10]"}},
}

// A result is one measurement: wall time and peak memory, or ok=false if the
// tool was not run on this workload.
type result struct {
	seconds float64
	mb      float64
	ok      bool
}

func (r result) String() string {
	if !r.ok {
		return "n/a"
	}
	return fmt.Sprintf("%5.2fs %4.0fMB", r.seconds, r.mb)
}

func run(file string, svgPath string) {
	pj := buildJP()
	tools := []string{"pj"}
	paths := []string{pj}
	for _, name := range jqLikeTools {
		if path, err := exec.LookPath(name); err == nil {
			tools = append(tools, name)
			paths = append(paths, path)
		} else {
			fmt.Fprintf(os.Stderr, "%s not found, skipping\n", name)
		}
	}
	// results[w][t] is the result of tool t on workload w.
	results := make([][]result, len(workloads))
	fmt.Printf("%-28s", "workload")
	for _, tool := range tools {
		fmt.Printf(" %13s", tool)
	}
	fmt.Println()
	for i, w := range workloads {
		results[i] = make([]result, len(tools))
		jpArgs := append([]string{"-color", "never"}, w.pj...)
		results[i][0] = measure(pj, append(jpArgs, file))
		for t := 1; t < len(tools); t++ {
			if w.jq != nil {
				results[i][t] = measure(paths[t], append(append([]string{}, w.jq...), file))
			}
		}
		fmt.Printf("%-28s", w.name)
		for _, r := range results[i] {
			fmt.Printf(" %13s", r)
		}
		fmt.Println()
	}
	if svgPath != "" {
		if err := os.WriteFile(svgPath, []byte(chartSVG(tools, results, file)), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "chart written to", svgPath)
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
		return result{}
	}
	rss := cmd.ProcessState.SysUsage().(*syscall.Rusage).Maxrss
	if runtime.GOOS != "darwin" {
		rss *= 1024 // Linux reports KB, macOS bytes
	}
	return result{seconds: elapsed.Seconds(), mb: float64(rss) / (1 << 20), ok: true}
}
