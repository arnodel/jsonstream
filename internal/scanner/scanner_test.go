package scanner

import (
	"strings"
	"testing"
)

func strScanner(s string) *Scanner {
	return NewScanner(strings.NewReader(s))
}

func assertRead(t *testing.T, s *Scanner, xb byte, xerr error) {
	b, err := s.Read()
	if b != xb {
		t.Fatalf("Read: expected b = %q, got %q", xb, b)
	}
	if err != xerr {
		t.Fatalf("Read: expected err = %s, got %s", xerr, err)
	}
}

func assertPeek(t *testing.T, s *Scanner, xb byte, xerr error) {
	b, err := s.Peek()
	if b != xb {
		t.Fatalf("Peek: expected b = %q, got %q", xb, b)
	}
	if err != xerr {
		t.Fatalf("Peek: expected err = %s, got %s", xerr, err)
	}
}

func assertCurrentPos(t *testing.T, s *Scanner, line, col int) {
	pos := s.CurrentPos()
	if pos.Line != line || pos.Col != col {
		t.Fatalf("CurrentPos: expected (%d, %d) got (%d, %d)", line, col, pos.Line, pos.Col)
	}
}

func assertStartToken(t *testing.T, s *Scanner, line, col int) {
	pos := s.StartToken()
	if pos.Line != line || pos.Col != col {
		t.Fatalf("StartToken: expected (%d, %d) got (%d, %d)", line, col, pos.Line, pos.Col)
	}
}

func assertEndToken(t *testing.T, s *Scanner, tokStr string) {
	tok := s.EndToken()
	if string(tok) != tokStr {
		t.Fatalf("EndToken: expected %q got %q", tokStr, tok)
	}
}

func TestSimple(t *testing.T) {
	scanner := strScanner("bonjour")
	assertRead(t, scanner, 'b', nil)
	assertRead(t, scanner, 'o', nil)
	assertCurrentPos(t, scanner, 0, 2)
	assertPeek(t, scanner, 'n', nil)
	assertCurrentPos(t, scanner, 0, 2)
	assertRead(t, scanner, 'n', nil)
	assertCurrentPos(t, scanner, 0, 3)
	scanner.Back()
	assertCurrentPos(t, scanner, 0, 2)
	assertRead(t, scanner, 'n', nil)
	assertCurrentPos(t, scanner, 0, 3)

	assertStartToken(t, scanner, 0, 3)
	assertRead(t, scanner, 'j', nil)
	assertRead(t, scanner, 'o', nil)
	assertRead(t, scanner, 'u', nil)
	assertRead(t, scanner, 'r', nil)
	assertCurrentPos(t, scanner, 0, 7)
	assertRead(t, scanner, EOF, nil)
	scanner.Back()
	assertRead(t, scanner, EOF, nil)
	assertCurrentPos(t, scanner, 0, 7)
	assertEndToken(t, scanner, "jour")
}

func TestLargeInput(t *testing.T) {
	const line = "A very long string.\n"
	scanner := NewScannerSize(strings.NewReader(strings.Repeat(line, 100)), 16)
	lc := 0
	// Check we get the correct bytes after the buffer is refilled.
	var acc []byte
	for lc < 10 {
		b, err := scanner.Read()
		if err != nil {
			t.Fatal("unexpected error")
		}
		acc = append(acc, b)
		if b == '\n' {
			lc++
		}
	}
	if string(acc) != strings.Repeat(line, 10) {
		t.Fatalf("incorrect input")
	}
	// Check tokens get put together correctly and everything is cleaned up
	// after each token is returned
	for i := 1; i <= 3; i++ {
		assertStartToken(t, scanner, 10*i, 0)
		lc = 0
		for lc < 10 {
			b, err := scanner.Read()
			if err != nil {
				t.Fatal("unexpected error")
			}
			acc = append(acc, b)
			if b == '\n' {
				lc++
			}
		}
		assertEndToken(t, scanner, strings.Repeat(line, 10))
	}
}
