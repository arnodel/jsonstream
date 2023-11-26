//go:build !debug

package token

type cursorPoolDebugData struct{}

func (p *CursorPool) checkWindowSize() {}
