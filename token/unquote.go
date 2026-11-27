package token

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"
)

// unquote returns the value of the JSON string literal b.  It decodes the
// escapes and keeps every other byte as it is, invalid UTF-8 included, so that
// a string has the same value whether or not it has escapes.  An escaped
// surrogate that is not part of a pair becomes U+FFFD.
func unquote(b []byte) string {
	body := b[1 : len(b)-1]
	i := bytes.IndexByte(body, '\\')
	if i < 0 {
		return string(body)
	}
	out := make([]byte, 0, len(body))
	for ; i >= 0; i = bytes.IndexByte(body, '\\') {
		out = append(out, body[:i]...)
		body = body[i:]
		if len(body) < 2 {
			break
		}
		switch c := body[1]; c {
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			r, n := unquoteRune(body)
			if n == 0 {
				out = append(out, body[:2]...)
				break
			}
			out = utf8.AppendRune(out, r)
			body = body[n-2:]
		default:
			out = append(out, c)
		}
		body = body[2:]
	}
	return string(append(out, body...))
}

// unquoteRune decodes the \u escape at the start of b, with the one after it
// if they make a surrogate pair, and returns the rune and the number of bytes
// read, or 0 if b does not start with a \u escape.
func unquoteRune(b []byte) (rune, int) {
	r, ok := hex4(b)
	if !ok {
		return 0, 0
	}
	if !utf16.IsSurrogate(r) {
		return r, 6
	}
	if r2, ok := hex4(b[6:]); ok && len(b) >= 12 && b[6] == '\\' {
		if pair := utf16.DecodeRune(r, r2); pair != utf8.RuneError {
			return pair, 12
		}
	}
	return utf8.RuneError, 6
}

// hex4 decodes the four hex digits of the \u escape at the start of b.
func hex4(b []byte) (rune, bool) {
	if len(b) < 6 || b[1] != 'u' {
		return 0, false
	}
	var r rune
	for _, c := range b[2:6] {
		switch {
		case '0' <= c && c <= '9':
			r = r<<4 | rune(c-'0')
		case 'a' <= c && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		case 'A' <= c && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return r, true
}
