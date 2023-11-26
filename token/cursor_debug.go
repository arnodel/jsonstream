//go:build debug

package token

import "log"

type cursorPoolDebugData struct {
	maxWindowSize int
}

func (p *CursorPool) checkWindowSize() {
	current := len(p.window)
	if current > p.maxWindowSize {
		p.maxWindowSize = current
		log.Printf("max window size = %d, pos = %d", current, p.windowPos)
	}
}
