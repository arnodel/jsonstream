package iregexp

import (
	"regexp"
	"strings"
)

func iregexpString(ptn string) string {
	inClass := false
	escape := false
	lastIndex := 0
	var builder strings.Builder
	for i, r := range ptn {
		if escape {
			escape = false
			continue
		}
		switch r {
		case '\\':
			escape = true
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '.':
			if !inClass {
				builder.WriteString(ptn[lastIndex:i])
				builder.WriteString(`[^\n\r]`)
				lastIndex = i + 1
			}
		}
	}
	if lastIndex == 0 {
		return ptn
	}
	if lastIndex < len(ptn) {
		builder.WriteString((ptn[lastIndex:]))
	}
	return builder.String()
}

// Compile compiles an I-Regexp (RFC 9485) pattern into a Go regular
// expression suitable for the JSONPath search() function: it matches anywhere
// in the subject.
func Compile(ptn string) (*regexp.Regexp, error) {
	return compileCached(ptn, false)
}

// CompileAnchored compiles an I-Regexp pattern so that it only matches the
// whole subject, as the JSONPath match() function requires.  Anchoring the
// pattern is the only reliable way to do this: finding the leftmost match and
// checking its extent fails for patterns such as a|ab on "ab".
func CompileAnchored(ptn string) (*regexp.Regexp, error) {
	return compileCached(ptn, true)
}

type cacheKey struct {
	ptn      string
	anchored bool
}

type cacheEntry struct {
	re  *regexp.Regexp
	err error
}

// Compiled patterns are cached because the generic function path compiles
// the pattern for every node.  Failures are cached too, so an invalid pattern
// is not recompiled for every node either.
var cache = map[cacheKey]cacheEntry{}

func compileCached(ptn string, anchored bool) (*regexp.Regexp, error) {
	key := cacheKey{ptn, anchored}
	if entry, ok := cache[key]; ok {
		return entry.re, entry.err
	}
	goPtn := iregexpString(ptn)
	if anchored {
		goPtn = "^(?:" + goPtn + ")$"
	}
	re, err := regexp.Compile(goPtn)
	cache[key] = cacheEntry{re, err}
	return re, err
}
