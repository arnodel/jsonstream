package parser

import (
	"encoding/json"
	"strconv"
	"strings"
)

func parseInt(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic(err)
	}
	return i
}

func parseDoubleQuotedString(s string) string {
	return parseJsonLiteral(s).(string)
}

func parseSingleQuotedString(s string) string {
	// Turn it into a double quoted string by
	// - Replacing the start and end quotes
	// - unescaping all single quotes
	// - escaping all double quotes
	s = strings.ReplaceAll(s[1:len(s)-1], `\'`, `\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = `"` + s + `"`
	return parseDoubleQuotedString(s)
}

func parseNumber(s string) float64 {
	return parseJsonLiteral(s).(float64)
}

func parseJsonLiteral(s string) json.Token {
	dec := json.NewDecoder(strings.NewReader(s))
	tok, err := dec.Token()
	if err != nil {
		panic(err)
	}
	return tok
}
