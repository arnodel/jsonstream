//go:build debug

package debug

import "log"

func Printf(msg string, args ...any) {
	log.Printf(msg, args...)
}

const On = true
