//go:build pprof

package main

import (
	"os"
	"runtime/pprof"
)

// startProfile writes a CPU profile of the run to the file PJ_CPUPROFILE
// names, if any, and returns the function that finishes it.  make pgo merges
// such profiles into default.pgo.
func startProfile() func() {
	path := os.Getenv("PJ_CPUPROFILE")
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path)
	if err == nil {
		err = pprof.StartCPUProfile(f)
	}
	if err != nil {
		fatalError("%s", err)
	}
	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}
}
