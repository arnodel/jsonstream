//go:build !pprof

package main

// startProfile does nothing: build with -tags pprof to write CPU profiles.
func startProfile() func() { return func() {} }
