//go:build !debug

package debug

func Printf(msg string, args ...any) {}

const On = false
