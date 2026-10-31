// Command bench generates a large JSON file and times jp, and whichever of jq,
// jaq and gojq are installed, on a few typical workloads.
//
//	go run ./bench gen 500000 > /tmp/big.json
//	go run ./bench run /tmp/big.json
//
// It reports wall-clock time and peak memory of each command, with output sent
// to /dev/null.  It is deliberately simple: one file shape, one run per
// workload, no statistics.  Run it a few times and look at the trend.
package main

import (
	"bufio"
	"encoding/json"
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
		run(os.Args[2])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:\n  bench gen N > file.json   generate a file with N items\n  bench run file.json       time jp, jq, jaq and gojq on it")
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

// A workload is a query in jp's syntax and the equivalent in jq's, which jaq
// and gojq share.
type workload struct {
	name string
	jp   []string
	jq   []string
}

// jqLikeTools are jq implementations with the same command line.
var jqLikeTools = []string{"jq", "jaq", "gojq"}

var workloads = []workload{
	{"pretty-print", []string{}, []string{"."}},
	{"JPV output", []string{"-out", "jpv"}, nil},
	{"$.items[*].name", []string{"$.items[*].name"}, []string{".items[].name"}},
	{"filter price > 190", []string{"-json-lines", "$.items[?@.price > 190]"}, []string{"-c", ".items[] | select(.price > 190)"}},
	{"first 10 items (file arg)", []string{"$.items[:10]"}, []string{".items[:10]"}},
}

func run(file string) {
	jp := buildJP()
	var tools []string
	for _, name := range jqLikeTools {
		if path, err := exec.LookPath(name); err == nil {
			tools = append(tools, path)
		} else {
			fmt.Fprintf(os.Stderr, "%s not found, skipping\n", name)
		}
	}
	fmt.Printf("%-28s %13s", "workload", "jp")
	for _, tool := range tools {
		fmt.Printf(" %13s", filepath.Base(tool))
	}
	fmt.Println()
	for _, w := range workloads {
		jpArgs := append([]string{"-color", "never"}, w.jp...)
		jpArgs = append(jpArgs, file)
		fmt.Printf("%-28s %13s", w.name, measure(jp, jpArgs))
		for _, tool := range tools {
			res := "n/a"
			if w.jq != nil {
				res = measure(tool, append(append([]string{}, w.jq...), file))
			}
			fmt.Printf(" %13s", res)
		}
		fmt.Println()
	}
}

func buildJP() string {
	path := filepath.Join(os.TempDir(), "jp-bench")
	cmd := exec.Command("go", "build", "-o", path, "./cmd/jp")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building jp:", err)
		os.Exit(1)
	}
	return path
}

// measure runs the command with output to /dev/null and returns "time memory".
func measure(name string, args []string) string {
	cmd := exec.Command(name, args...)
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	defer devnull.Close()
	cmd.Stdout = devnull
	cmd.Stderr = os.Stderr
	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)
	if err != nil {
		return "error"
	}
	rss := cmd.ProcessState.SysUsage().(*syscall.Rusage).Maxrss
	if runtime.GOOS != "darwin" {
		rss *= 1024 // Linux reports KB, macOS bytes
	}
	return fmt.Sprintf("%5.2fs %4dMB", elapsed.Seconds(), rss/(1<<20))
}
