package jsonpath

import (
	"regexp"

	"github.com/arnodel/jsonstream/internal/jsonpath/iregexp"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

// regexFunctionRunner evaluates match() or search() when the pattern is a
// literal, which is nearly always.  The regular expression is compiled once
// when the query is compiled instead of once per node.
type regexFunctionRunner struct {
	subject FunctionArgumentRunner

	// re is nil when the pattern is not a valid I-Regexp, in which case the
	// function is false for every node, as it is on the generic path.  For
	// match() it is anchored so that only the whole subject can match.
	re *regexp.Regexp
}

var _ FunctionArgumentRunner = regexFunctionRunner{}

func newRegexFunctionRunner(subject FunctionArgumentRunner, pattern string, fullMatch bool) regexFunctionRunner {
	var re *regexp.Regexp
	var err error
	if fullMatch {
		re, err = iregexp.CompileAnchored(pattern)
	} else {
		re, err = iregexp.Compile(pattern)
	}
	if err != nil {
		re = nil
	}
	return regexFunctionRunner{subject: subject, re: re}
}

func (r regexFunctionRunner) EvaluateTruth(ctx *RunContext, val iterator.Value) bool {
	if r.re == nil {
		return false
	}
	clone, detach := val.Clone()
	if detach != nil {
		defer detach()
	}
	s, ok := r.subject.Evaluate(ctx, clone).(*iterator.Scalar)
	if !ok || s.Scalar().Type() != token.String {
		return false
	}
	if s.Scalar().IsUnescaped() {
		return r.re.Match(s.Bytes[1 : len(s.Bytes)-1])
	}
	return r.re.MatchString(s.Scalar().ToString())
}

func (r regexFunctionRunner) Evaluate(ctx *RunContext, val iterator.Value) iterator.Value {
	panic("invalid Evaluate call: match and search are logical")
}

func (r regexFunctionRunner) EvaluateNodesResult(ctx *RunContext, val iterator.Value) NodesResult {
	panic("invalid EvaluateNodesResult call: match and search are logical")
}
