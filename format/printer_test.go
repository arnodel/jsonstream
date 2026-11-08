package format

import (
	"strings"
	"testing"
)

type countingFlusher struct{ n int }

func (f *countingFlusher) Flush() error { f.n++; return nil }

func TestDefaultPrinterIndentation(t *testing.T) {
	var b strings.Builder
	var f countingFlusher
	p := &DefaultPrinter{Writer: &b, Flusher: &f, IndentSize: 3}
	p.PrintBytes([]byte("a"))
	// Go deeper than any line so far, so the indentation has to grow.
	for range 5 {
		p.Indent()
	}
	p.PrintBytes([]byte("b"))
	p.Dedent()
	p.PrintBytes([]byte("c"))
	p.NewLine()
	p.PrintBytes([]byte("d"))
	p.Reset()
	p.PrintBytes([]byte("e"))
	p.NewLine()

	var want strings.Builder
	want.WriteString("a\n")
	for i := 1; i < 5; i++ {
		want.WriteString(strings.Repeat(" ", 3*i) + "\n")
	}
	want.WriteString(strings.Repeat(" ", 15) + "b\n")
	want.WriteString(strings.Repeat(" ", 12) + "c\n")
	want.WriteString(strings.Repeat(" ", 12) + "d\n")
	want.WriteString("e\n")
	if b.String() != want.String() {
		t.Errorf("got %q, want %q", b.String(), want.String())
	}
	if lines := strings.Count(b.String(), "\n"); f.n != lines {
		t.Errorf("flushed %d times for %d lines", f.n, lines)
	}
}

func TestDefaultPrinterSingleLine(t *testing.T) {
	var b strings.Builder
	p := &DefaultPrinter{Writer: &b, IndentSize: -1}
	p.PrintBytes([]byte("["))
	p.Indent()
	p.PrintBytes([]byte("1"))
	p.Dedent()
	p.PrintBytes([]byte("]"))
	p.Reset()
	if b.String() != "[1]\n" {
		t.Errorf("got %q", b.String())
	}
}
