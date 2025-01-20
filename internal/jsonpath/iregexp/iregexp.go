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

func Compile(ptn string) (*regexp.Regexp, error) {
	re, ok := knownIRegexps[ptn]
	if ok {
		return re, nil
	}
	ptn = iregexpString(ptn)
	re, err := regexp.Compile(ptn)
	if err != nil {
		knownIRegexps[ptn] = re
	}
	return re, err
}

var knownIRegexps = map[string](*regexp.Regexp){}
