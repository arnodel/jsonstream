package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	pgoItems   = 200000          // items in the generated files, about 30 MB of JSON
	pgoSpendOn = 3 * time.Second // time to run each command for
)

// pgo profiles pj on the commands of pgoCommands and merges the profiles into
// out, the profile that go build uses for a profile-guided build when it is
// cmd/pj/default.pgo.
func pgo(out string) {
	dir, err := os.MkdirTemp("", "pj-pgo")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	// Built without the current profile, so that the new one does not
	// depend on it.
	pj := buildPJ(filepath.Join(dir, "pj"), "-tags", "pprof", "-pgo", "off")
	json := filepath.Join(dir, "data.json")
	writeGen(json, pgoItems)
	writeFlat([]string{"pj"}, []string{pj}, json)
	csv := filepath.Join(dir, "data.csv")
	writeCSV(csv, pgoItems)

	var profiles []string
	for i, args := range pgoCommands(json, flatFile("pj", json), csv) {
		// Each command runs for about the same time, so that it weighs about
		// as much as the others in the merged profile.
		runs := 0
		for spent := time.Duration(0); spent < pgoSpendOn; runs++ {
			profile := filepath.Join(dir, fmt.Sprintf("%d-%d.pprof", i, runs))
			cmd := exec.Command(pj, args...)
			cmd.Env = append(os.Environ(), "PJ_CPUPROFILE="+profile)
			cmd.Stderr = os.Stderr
			start := time.Now()
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "pj %s: %v\n", shellJoin(args), err)
				os.Exit(1)
			}
			spent += time.Since(start)
			profiles = append(profiles, profile)
		}
		fmt.Fprintf(os.Stderr, "%3d runs: pj %s\n", runs, strings.ReplaceAll(shellJoin(args), dir+string(filepath.Separator), ""))
	}

	f, err := os.Create(out)
	if err == nil {
		cmd := exec.Command("go", append([]string{"tool", "pprof", "-proto"}, profiles...)...)
		cmd.Stdout = f
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "writing %s: %v\n", out, err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "profile written to", out)
}

// pgoCommands are the pj command lines profiled for default.pgo: the
// benchmark's queries, except the early exit, which is mostly start-up, and
// each format pj reads and writes, with colour as in a terminal too.
func pgoCommands(json, jpv, csv string) [][]string {
	var cmds [][]string
	for _, w := range suites[0].workloads {
		if w.name != "first 10 items" {
			cmds = append(cmds, w.args("pj", json))
		}
	}
	return append(cmds,
		[]string{"-color", "always", json},
		[]string{"-color", "never", "-json-lines", "$.items[*]", json},
		[]string{"-color", "never", "-out", "jpv", json},
		[]string{"-color", "never", "-in", "jpv", jpv},
		[]string{"-color", "never", "-in", "csvh", csv},
	)
}

// writeCSV writes a CSV file with a header and n records like gen's items.
func writeCSV(path string, n int) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rnd := rand.New(rand.NewSource(1))
	out := bufio.NewWriterSize(f, 1<<20)
	out.WriteString("id,name,email,active,price,city\n")
	for i := range n {
		fmt.Fprintf(out, "%d,user%d,user%d@example.com,%t,%.2f,Boston\n", i, i, i, i%3 == 0, float64(int(rnd.Float64()*20000))/100)
	}
	err = out.Flush()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
