package parser

import (
	"testing"

	"github.com/arnodel/grammar"
)

func tok(tp, val string) Token {
	return grammar.SimpleToken{TokType: tp, TokValue: val}
}

func tokp(tp, val string) *Token {
	return &grammar.SimpleToken{TokType: tp, TokValue: val}
}

func checkStream(t *testing.T, stream grammar.TokenStream, expectedTokens []Token) {
	for i, expectedTok := range expectedTokens {
		tok := stream.Next()
		if tok.Type() != expectedTok.Type() {
			t.Fatalf("Token %d: expected type %q, got %q", i, expectedTok.Type(), tok.Type())
		}
		if tok.Value() != expectedTok.Value() {
			t.Fatalf("Token %d: expected value %q, got %q", i, expectedTok.Value(), tok.Value())
		}
	}
}

func TestLexer(t *testing.T) {
	var tests = []struct {
		name   string
		input  string
		tokens []Token
		err    error
	}{
		{
			name:  "simple",
			input: `$.foo["hello"]..['bar 1']`,
			tokens: []Token{
				tok("op", "$"),
				tok("membernameshorthand", ".foo"),
				tok("op", "["),
				tok("doublequotedstring", `"hello"`),
				tok("op", "]"),
				tok("op", "..["),
				tok("singlequotedstring", `'bar 1'`),
				tok("op", "]"),
				grammar.EOF,
			},
		},
		{
			name:  "example 1",
			input: "$.store.book[*].author",
			tokens: []Token{
				tok("op", "$"),
				tok("membernameshorthand", ".store"),
				tok("membernameshorthand", ".book"),
				tok("op", "["),
				tok("op", "*"),
				tok("op", "]"),
				tok("membernameshorthand", ".author"),
				grammar.EOF,
			},
		},
		{
			name:  "example 2",
			input: "$..author",
			tokens: []Token{
				tok("op", "$"),
				tok("descendantmembernameshorthand", "..author"),
				grammar.EOF,
			},
		},
		{
			name:  "example 3",
			input: "$.store.*",
			tokens: []Token{
				tok("op", "$"),
				tok("membernameshorthand", ".store"),
				tok("op", ".*"),
				grammar.EOF,
			},
		},
		{
			name:  "example 4",
			input: "$.store..price",
			tokens: []Token{
				tok("op", "$"),
				tok("membernameshorthand", ".store"),
				tok("descendantmembernameshorthand", "..price"),
				grammar.EOF,
			},
		},
		{
			name:  "example 5",
			input: "$..book[2]",
			tokens: []Token{
				tok("op", "$"),
				tok("descendantmembernameshorthand", "..book"),
				tok("op", "["),
				tok("int", "2"),
				tok("op", "]"),
				grammar.EOF,
			},
		},
		{
			name:  "example 6",
			input: "$..book[2].author",
			tokens: []Token{
				tok("op", "$"),
				tok("descendantmembernameshorthand", "..book"),
				tok("op", "["),
				tok("int", "2"),
				tok("op", "]"),
				tok("membernameshorthand", ".author"),
				grammar.EOF,
			},
		},
		{
			name:  "example 7",
			input: "$..book[2].publisher",
			// Skip this one
		},
		{
			name:  "example 8",
			input: "$..book[-1]",
			tokens: []Token{
				tok("op", "$"),
				tok("descendantmembernameshorthand", "..book"),
				tok("op", "["),
				tok("int", "-1"),
				tok("op", "]"),
				grammar.EOF,
			},
		},
		{
			name:  "example 9a",
			input: "$..book[0,1]",
			tokens: []Token{
				tok("op", "$"),
				tok("descendantmembernameshorthand", "..book"),
				tok("op", "["),
				tok("int", "0"),
				tok("op", ","),
				tok("int", "1"),
				tok("op", "]"),
				grammar.EOF,
			},
		},
		{
			name:  "example 9b",
			input: "$..book[:2]",
			tokens: []Token{
				tok("op", "$"),
				tok("descendantmembernameshorthand", "..book"),
				tok("op", "["),
				tok("op", ":"),
				tok("int", "2"),
				tok("op", "]"),
				grammar.EOF,
			},
		},
		{
			name:  "example 10",
			input: "$..book[?@.isbn]",
			tokens: []Token{
				tok("op", "$"),
				tok("descendantmembernameshorthand", "..book"),
				tok("op", "["),
				tok("op", "?"),
				tok("op", "@"),
				tok("membernameshorthand", ".isbn"),
				tok("op", "]"),
				grammar.EOF,
			},
		},
		{
			name:  "example 11",
			input: "$..book[?@.price < 10]",
			tokens: []Token{
				tok("op", "$"),
				tok("descendantmembernameshorthand", "..book"),
				tok("op", "["),
				tok("op", "?"),
				tok("op", "@"),
				tok("membernameshorthand", ".price"),
				tok("comparisonop", "<"),
				tok("int", "10"),
				tok("op", "]"),
				grammar.EOF,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream, err := TokeniseJsonPathString(test.input)
			if err != test.err {
				t.Fatalf("Error is %s - expected %s", err, test.err)
			}
			checkStream(t, stream, test.tokens)
		})
	}
}
