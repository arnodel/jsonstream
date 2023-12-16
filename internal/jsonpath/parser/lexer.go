package parser

import "github.com/arnodel/grammar"

var TokeniseJsonPathString = grammar.SimpleTokeniser([]grammar.TokenDef{
	{
		Ptn: `\s+`,
	},
	{
		Name: "null",
		Ptn:  `null\b`,
	},
	{
		Name: "bool",
		Ptn:  `true\b|false\b`,
	},
	{
		Name: "functionname",
		Ptn:  `[a-z][a-z_0-9]*\b`,
	},
	{
		Name: "comparisonop",
		Ptn:  `==|!=|<=|>=|<|>`,
	},
	{
		Name: "descendantmembernameshorthand",
		Ptn:  `\.\.[a-zA-Z_\x80-\x{D7FF}\x{E000}-\x{10FFFF}][0-9a-zA-Z_\x80-\x{D7FF}\x{E000}-\x{10FFFF}]*`,
	},
	{
		Name: "membernameshorthand",
		Ptn:  `\.[a-zA-Z_\x80-\x{D7FF}\x{E000}-\x{10FFFF}][0-9a-zA-Z_\x80-\x{D7FF}\x{E000}-\x{10FFFF}]*`,
	},
	{
		Name: "op",
		Ptn:  `&&|\|\||\.\.[*[]|\.\*|[$*:?!()@[\],]`,
	},
	{
		Name: "int",
		Ptn:  `(?:0|-?[1-9][0-9]*)(?:[^.e0-9]|$)`,
		Special: func(input string) string {
			i := 0
			if input[i] == '-' {
				i++
			}
			for ; i < len(input); i++ {
				if input[i] > '9' || input[i] < '0' {
					break
				}
			}
			return input[:i]
		},
	},
	{
		Name: "number",
		Ptn:  `(?:-?0|-?[1-9][0-9]*)(?:\.[0-9]+)?(?:e[+-]?[0-9]+)?\b`,
	},
	{
		Name: "doublequotedstring",
		Ptn:  `"(?:\\[bfnrt/\\"]|\\u[0-9ABCEFabcef][0-9A-Fa-f]{3}|\\uD[89ABab][0-9A-Fa-f]{2}\\u[Dd][C-Fc-f][0-9A-Fa-f]{2}|[\x20-\x21\x23-\x5B\x5D-\x{D7FF}\x{E000}-\x{10FFFF}])*"`,
	},
	{
		Name: "singlequotedstring",
		Ptn:  `'(?:\\[bfnrt/\\']|\\u[0-9ABCEFabcef][0-9A-Fa-f]{3}|\\uD[89ABab][0-9A-Fa-f]{2}\\u[Dd][C-Fc-f][0-9A-Fa-f]{2}|[\x20-\x26\x28-\x5B\x5D-\x{D7FF}\x{E000}-\x{10FFFF}])*'`,
	},
})
