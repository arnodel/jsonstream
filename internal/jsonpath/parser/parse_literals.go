package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	maxSafeInt = 9007199254740991
	minSafeInt = -9007199254740991
)

func parseInt(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return n, err
	}
	if n > maxSafeInt || n < minSafeInt {
		return n, fmt.Errorf("out of safe bounds integer value: %d", n)
	}
	return n, nil
}

func parseDoubleQuotedString(s string) (string, error) {
	tok, err := ParseJsonLiteral(s)
	if err != nil {
		return "", err
	}
	return tok.(string), err
}

func parseSingleQuotedString(s string) (string, error) {
	// Turn it into a double quoted string by
	// - Replacing the start and end quotes
	// - unescaping all single quotes
	// - escaping all double quotes
	s = strings.ReplaceAll(s[1:len(s)-1], `\'`, `'`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = `"` + s + `"`
	return parseDoubleQuotedString(s)
}

func parseNumber(s string) (float64, error) {
	tok, err := ParseJsonLiteral(s)
	if err != nil {
		return 0, err
	}
	return tok.(float64), nil
}

func ParseJsonLiteral(s string) (json.Token, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	return dec.Token()
}

func ParseJsonLiteralBytes(b []byte) json.Token {
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		panic(err)
	}
	return tok
}
