package parser

import "github.com/arnodel/grammar"

// The grammar is parsed by trying alternatives in order (a PEG), and several
// alternatives can start with the same rule: a function call can begin a
// comparison or a test expression, and a function argument is tried as a
// function call and then as a logical expression, which tries both again.
// When such a rule fails, each alternative parses it again from the same
// token, and the work multiplies at every level of nesting: an unclosed
// "$[?a(a(a(a(a(a(a(a(a(a(" took seconds to reject.
//
// A rule's outcome depends only on where it starts, so a failure can be
// remembered and returned straight away the next time the rule is tried at
// that position.  Only failures are remembered: a rule that succeeds is
// committed to by the alternative that tried it, so it is not parsed again
// from the same position in a way that multiplies.  Remembered errors also
// have their lists of expected tokens deduplicated (see dedupOptions).

// A TokenStream is a SimpleTokenStream that remembers where rules have failed
// to parse.  Use NewTokenStream to make one for grammar.Parse.
type TokenStream struct {
	*grammar.SimpleTokenStream
	failures map[failureKey]*grammar.ParseError
}

type failureKey struct {
	rule ruleID
	pos  int
}

type ruleID int

const (
	functionExprRule ruleID = iota
)

// NewTokenStream returns a TokenStream reading from s.
func NewTokenStream(s *grammar.SimpleTokenStream) *TokenStream {
	return &TokenStream{SimpleTokenStream: s}
}

// parseRemembering calls parse unless rule is known to fail at the current
// position, and remembers if it fails.  It only does so for rules parsed
// without token options (none of the remembered rules have any) on a
// TokenStream.
func parseRemembering(rule ruleID, s *grammar.ParserState, opts grammar.ParseOptions, parse func() *grammar.ParseError) *grammar.ParseError {
	ts, ok := s.TokenStream.(*TokenStream)
	if !ok || len(opts.TokenParseOptions) != 0 {
		return parse()
	}
	key := failureKey{rule: rule, pos: ts.Save()}
	if err, failed := ts.failures[key]; failed {
		return err
	}
	err := parse()
	if err != nil {
		err = dedupOptions(err)
		if ts.failures == nil {
			ts.failures = map[failureKey]*grammar.ParseError{}
		}
		ts.failures[key] = err
	}
	return err
}

// dedupOptions returns err with each expected token listed once.  Errors at
// the same position are merged by concatenating what they expected, and
// when everything fails at the same token (as when a query is cut short)
// every level of nesting multiplies the list.
func dedupOptions(err *grammar.ParseError) *grammar.ParseError {
	seen := make(map[grammar.TokenParseOptions]bool, len(err.TokenParseOptions))
	var opts []grammar.TokenParseOptions
	for _, o := range err.TokenParseOptions {
		if !seen[o] {
			seen[o] = true
			opts = append(opts, o)
		}
	}
	if len(opts) == len(err.TokenParseOptions) {
		return err
	}
	dedup := *err
	dedup.TokenParseOptions = opts
	return &dedup
}

// Parse parses a function call, remembering failures (see TokenStream).
//
// Rules containing a FunctionExpr must use a named field rather than embed
// it: an embedded FunctionExpr would give them this Parse method as well as
// their own grammar.OneOf or grammar.Seq one, and so neither.
func (e *FunctionExpr) Parse(r interface{}, s *grammar.ParserState, opts grammar.ParseOptions) *grammar.ParseError {
	return parseRemembering(functionExprRule, s, opts, func() *grammar.ParseError {
		return e.Seq.Parse(r, s, opts)
	})
}
