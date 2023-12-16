// Package parser implements a parser for jsonpath.
//
// Grammar taken from https://datatracker.ietf.org/doc/draft-ietf-jsonpath-base/21/
package parser

import (
	"fmt"

	"github.com/arnodel/grammar"
	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
)

type Token = grammar.SimpleToken

type Query struct {
	grammar.Seq
	RootIdentifier Token `tok:"op,$"`
	Segments       []Segment
}

func (q *Query) CompileToQuery() (ast.Query, error) {
	var compiledSegments = make([]ast.Segment, len(q.Segments))
	for i, s := range q.Segments {
		segment, err := s.CompileToSegment()
		if err != nil {
			return ast.Query{}, err
		}
		compiledSegments[i] = segment
	}
	return ast.Query{
		RootNode: ast.RootNodeIdentifier,
		Segments: compiledSegments,
	}, nil
}

type Selector struct {
	grammar.OneOf
	NameSelector     *StringLiteral
	WildcardSelector *Token `tok:"op,*"`
	*SliceSelector
	IndexSelector *Token `tok:"int"`
	*FilterSelector
}

func (s *Selector) CompileToSelector() (ast.Selector, error) {
	switch {
	case s.NameSelector != nil:
		name, err := s.NameSelector.CompileToString()
		if err != nil {
			return nil, err
		}
		return ast.NameSelector{Name: name}, nil
	case s.WildcardSelector != nil:
		return ast.WildcardSelector{}, nil
	case s.SliceSelector != nil:
		return s.SliceSelector.CompileToSelector()
	case s.IndexSelector != nil:
		index, err := parseInt(s.IndexSelector.TokValue)
		if err != nil {
			return nil, fmt.Errorf("invalid index; %w", err)
		}
		return ast.IndexSelector{Index: index}, nil
	case s.FilterSelector != nil:
		return s.FilterSelector.CompileToSelector()
	default:
		panic("invalid Selector")
	}
}

type StringLiteral struct {
	grammar.OneOf
	DoubleQuotedString *Token `tok:"doublequotedstring"`
	SingleQuotedString *Token `tok:"singlequotedstring"`
}

func (s *StringLiteral) CompileToString() (string, error) {
	switch {
	case s.DoubleQuotedString != nil:
		return parseDoubleQuotedString(s.DoubleQuotedString.TokValue)
	case s.SingleQuotedString != nil:
		return parseSingleQuotedString(s.SingleQuotedString.TokValue)
	default:
		panic("invalid StringLiteral")
	}
}

type SliceSelector struct {
	grammar.Seq
	Start *Token `tok:"int"`
	Colon Token  `tok:"op,:"`
	End   *Token `tok:"int"`
	*SliceStep
}

func (s *SliceSelector) CompileToSelector() (ast.Selector, error) {
	var start, end *int64
	var step int64 = 1
	if s.Start != nil {
		startInt, err := parseInt(s.Start.TokValue)
		if err != nil {
			return nil, fmt.Errorf("invalid start index: %w", err)
		}
		start = &startInt
	}
	if s.End != nil {
		endInt, err := parseInt(s.End.TokValue)
		if err != nil {
			return nil, fmt.Errorf("invalid end index: %w", err)
		}
		end = &endInt
	}
	if s.SliceStep != nil {
		var err error
		step, err = parseInt(s.Step.TokValue)
		if err != nil {
			return nil, fmt.Errorf("invalid step index: %w", err)
		}
	}
	return ast.SliceSelector{Start: start, End: end, Step: step}, nil
}

type SliceStep struct {
	grammar.Seq
	StepColon Token `tok:"op,:"`
	Step      Token `tok:"int"`
}

type FilterSelector struct {
	grammar.Seq
	FilterIdentifier Token `tok:"op,?"`
	LogicalExpr
}

func (s *FilterSelector) CompileToSelector() (ast.Selector, error) {
	cond, err := s.LogicalExpr.CompileToLogicalExpr()
	if err != nil {
		return nil, err
	}
	return ast.FilterSelector{Condition: cond}, err
}

type LogicalExpr struct {
	grammar.Seq
	First LogicalAndExpr
	Rest  []LogicalOrRest
}

func (e *LogicalExpr) CompileToLogicalExpr() (ast.LogicalExpr, error) {
	first, err := e.First.CompileToLogicalExpr()
	if err != nil {
		return nil, err
	}
	if len(e.Rest) == 0 {
		return first, nil
	}
	var args = make([]ast.LogicalExpr, len(e.Rest)+1)
	args[0] = first
	for i, t := range e.Rest {
		arg, err := t.Operand.CompileToLogicalExpr()
		if err != nil {
			return nil, err
		}
		args[i+1] = arg
	}
	return ast.OrExpr{Arguments: args}, nil
}

type LogicalOrRest struct {
	grammar.Seq
	Or      Token `tok:"op,||"`
	Operand LogicalAndExpr
}

type LogicalAndExpr struct {
	grammar.Seq
	First BasicExpr
	Rest  []LogicalAndRest
}

func (e *LogicalAndExpr) CompileToLogicalExpr() (ast.LogicalExpr, error) {
	first, err := e.First.CompileToLogicalExpr()
	if err != nil {
		return nil, err
	}
	if len(e.Rest) == 0 {
		return first, nil
	}
	var args = make([]ast.LogicalExpr, len(e.Rest)+1)
	args[0] = first
	for i, t := range e.Rest {
		arg, err := t.Operand.CompileToLogicalExpr()
		if err != nil {
			return nil, err
		}
		args[i+1] = arg
	}
	return ast.AndExpr{Arguments: args}, nil
}

type LogicalAndRest struct {
	grammar.Seq
	And     Token `tok:"op,&&"`
	Operand BasicExpr
}

type BasicExpr struct {
	grammar.OneOf
	*ParenExpr
	*ComparisonExpr
	*TestExpr
}

func (e *BasicExpr) CompileToLogicalExpr() (ast.LogicalExpr, error) {
	switch {
	case e.ParenExpr != nil:
		return e.ParenExpr.CompileToLogicalExpr()
	case e.ComparisonExpr != nil:
		return e.ComparisonExpr.CompileToLogicalExpr()
	case e.TestExpr != nil:
		return e.TestExpr.CompileToLogicalExpr()
	default:
		panic("invalid BasicExpr")
	}
}

type ParenExpr struct {
	grammar.Seq
	Not       *Token `tok:"op,!"`
	OpenParen Token  `tok:"op,("`
	LogicalExpr
	CloseParen Token `tok:"op,)"`
}

func (e *ParenExpr) CompileToLogicalExpr() (ast.LogicalExpr, error) {
	innerExpr, err := e.LogicalExpr.CompileToLogicalExpr()
	if err != nil {
		return nil, err
	}
	if e.Not != nil {
		return ast.NotExpr{Argument: innerExpr}, nil
	}
	return innerExpr, nil
}

type TestExpr struct {
	grammar.Seq
	Not *Token `tok:"op,!"`
	BasicTestExpr
}

func (e *TestExpr) CompileToLogicalExpr() (ast.LogicalExpr, error) {
	innerExpr, err := e.BasicTestExpr.CompileToLogicalExpr()
	if err != nil {
		return nil, err
	}
	if e.Not != nil {
		return ast.NotExpr{Argument: innerExpr}, nil
	}
	return innerExpr, nil
}

type BasicTestExpr struct {
	grammar.OneOf
	*FilterQuery
	*FunctionExpr
}

func (e *BasicTestExpr) CompileToLogicalExpr() (ast.LogicalExpr, error) {
	switch {
	case e.FilterQuery != nil:
		return e.FilterQuery.CompileToQuery()
	case e.FunctionExpr != nil:
		return e.FunctionExpr.CompileToFunctionExpr()
	default:
		panic("invalid BasicTestExpr")
	}
}

type FilterQuery struct {
	grammar.OneOf
	*RelQuery
	*Query
}

func (q *FilterQuery) CompileToQuery() (ast.Query, error) {
	switch {
	case q.RelQuery != nil:
		return q.RelQuery.CompileToQuery()
	case q.Query != nil:
		return q.Query.CompileToQuery()
	default:
		panic("invalid FilterQuery")
	}
}

type RelQuery struct {
	grammar.Seq
	CurrentNodeIdentifier Token `tok:"op,@"`
	Segments              []Segment
}

func (q *RelQuery) CompileToQuery() (ast.Query, error) {
	var compiledSegments = make([]ast.Segment, len(q.Segments))
	for i, s := range q.Segments {
		segment, err := s.CompileToSegment()
		if err != nil {
			return ast.Query{}, err
		}
		compiledSegments[i] = segment
	}
	return ast.Query{
		RootNode: ast.CurrentNodeIdentifier,
		Segments: compiledSegments,
	}, nil
}

type ComparisonExpr struct {
	grammar.Seq
	Left  Comparable
	Op    Token `tok:"comparisonop"`
	Right Comparable
}

func (e *ComparisonExpr) CompileToLogicalExpr() (ast.LogicalExpr, error) {
	var op ast.ComparisonOp
	switch e.Op.TokValue {
	case "==":
		op = ast.EqualOp
	case "!=":
		op = ast.NotEqualOp
	case "<=":
		op = ast.LessThanOrEqualOp
	case ">=":
		op = ast.GreaterThanOrEqualOp
	case "<":
		op = ast.LessThanOp
	case ">":
		op = ast.GreaterThanOp
	}
	left, err := e.Left.CompileToComparable()
	if err != nil {
		return nil, err
	}
	right, err := e.Right.CompileToComparable()
	if err != nil {
		return nil, err
	}
	return ast.ComparisonExpr{
		Left:  left,
		Op:    op,
		Right: right,
	}, nil
}

type Comparable struct {
	grammar.OneOf
	*Literal
	*SingularQuery
	*FunctionExpr
}

func (c *Comparable) CompileToComparable() (ast.Comparable, error) {
	switch {
	case c.Literal != nil:
		return c.Literal.CompileToLiteral()
	case c.SingularQuery != nil:
		return c.SingularQuery.CompileToSingularQuery()
	case c.FunctionExpr != nil:
		return c.FunctionExpr.CompileToFunctionExpr()
	default:
		panic("invalid Comparable")
	}
}

type Literal struct {
	grammar.OneOf
	Number *Token `tok:"int|number"`
	*StringLiteral
	Boolean *Token `tok:"bool"`
	Null    *Token `tok:"null"`
}

func (l *Literal) CompileToLiteral() (ast.Literal, error) {
	switch {
	case l.Number != nil:
		number, err := parseNumber(l.Number.TokValue)
		if err != nil {
			return ast.Literal{}, err
		}
		return ast.Literal{Value: number}, nil
	case l.StringLiteral != nil:
		value, err := l.StringLiteral.CompileToString()
		if err != nil {
			return ast.Literal{}, err
		}
		return ast.Literal{Value: value}, nil
	case l.Boolean != nil:
		switch l.Boolean.TokValue {
		case "true":
			return ast.Literal{Value: true}, nil
		case "false":
			return ast.Literal{Value: false}, nil
		default:
			panic("invalid Literal.Bool")
		}
	case l.Null != nil:
		return ast.Literal{}, nil
	default:
		panic("invalid Literal")
	}
}

type SingularQuery struct {
	grammar.OneOf
	*RelSingularQuery
	*AbsSingularQuery
}

func (q *SingularQuery) CompileToSingularQuery() (ast.SingularQuery, error) {
	switch {
	case q.RelSingularQuery != nil:
		return q.RelSingularQuery.CompileToSingularQuery()
	case q.AbsSingularQuery != nil:
		return q.AbsSingularQuery.CompileToSingularQuery()
	default:
		panic("invalid SingularQuery")
	}
}

type RelSingularQuery struct {
	grammar.Seq
	CurrentNodeIdentifier Token `tok:"op,@"`
	Segments              []SingularQuerySegment
}

func (q *RelSingularQuery) CompileToSingularQuery() (ast.SingularQuery, error) {
	var segments = make([]ast.SingularQuerySegment, len(q.Segments))
	for i, s := range q.Segments {
		segment, err := s.CompileToSingularQuerySegment()
		if err != nil {
			return ast.SingularQuery{}, err
		}
		segments[i] = segment
	}
	return ast.SingularQuery{
		RootNode: ast.CurrentNodeIdentifier,
		Segments: segments,
	}, nil
}

type AbsSingularQuery struct {
	grammar.Seq
	RootIdentifier Token `tok:"op,$"`
	Segments       []SingularQuerySegment
}

func (q *AbsSingularQuery) CompileToSingularQuery() (ast.SingularQuery, error) {
	var segments = make([]ast.SingularQuerySegment, len(q.Segments))
	for i, s := range q.Segments {
		segment, err := s.CompileToSingularQuerySegment()
		if err != nil {
			return ast.SingularQuery{}, fmt.Errorf("segment %d invalid: %w", i+1, err)
		}
		segments[i] = segment
	}
	return ast.SingularQuery{
		RootNode: ast.RootNodeIdentifier,
		Segments: segments,
	}, nil
}

type SingularQuerySegment struct {
	grammar.OneOf
	*BracketNameSegment
	DotNameSegment *Token `tok:"membernameshorthand"`
	*IndexSegment
}

func (s *SingularQuerySegment) CompileToSingularQuerySegment() (ast.SingularQuerySegment, error) {
	switch {
	case s.BracketNameSegment != nil:
		return s.BracketNameSegment.CompileToSingularQuerySegment()
	case s.DotNameSegment != nil:
		// S.DotNameSegment is of the form '.name'
		return ast.NameSegment{Name: s.DotNameSegment.TokValue[1:]}, nil
	case s.IndexSegment != nil:
		return s.IndexSegment.CompileToSingularQuerySegment()
	default:
		panic("invalid SingularQuerySegment")
	}
}

type BracketNameSegment struct {
	grammar.Seq
	OpenSquareBracket  Token `tok:"op,["`
	NameSelector       StringLiteral
	CloseSquareBracket Token `tok:"op,]"`
}

func (s *BracketNameSegment) CompileToSingularQuerySegment() (ast.SingularQuerySegment, error) {
	name, err := s.NameSelector.CompileToString()
	if err != nil {
		return nil, err
	}
	return ast.NameSegment{Name: name}, nil
}

type IndexSegment struct {
	grammar.Seq
	OpenSquareBracket  Token `tok:"op,["`
	IndexSelector      Token `tok:"int"`
	CloseSquareBracket Token `tok:"op,]"`
}

func (s *IndexSegment) CompileToSingularQuerySegment() (ast.SingularQuerySegment, error) {
	index, err := parseInt(s.IndexSelector.TokValue)
	if err != nil {
		return nil, fmt.Errorf("invalid index: %w", err)
	}
	return ast.IndexSegment{Index: index}, nil
}

type FunctionExpr struct {
	grammar.Seq
	FunctionNameAndOpenBracket Token `tok:"functionname("`
	Arguments                  *FunctionExprArguments
	CloseBracket               Token `tok:"op,)"`
}

func (e *FunctionExpr) CompileToFunctionExpr() (ast.FunctionExpr, error) {
	args, err := e.Arguments.CompileToFunctionArguments()
	if err != nil {
		return ast.FunctionExpr{}, nil
	}
	fname := e.FunctionNameAndOpenBracket.TokValue
	return ast.FunctionExpr{
		FunctionName: fname[:len(fname)-1],
		Arguments:    args,
	}, nil
}

type FunctionExprArguments struct {
	grammar.Seq
	First FunctionArgument
	Rest  []FunctionExprArgumentsRest
}

func (a *FunctionExprArguments) CompileToFunctionArguments() ([]ast.FunctionArgument, error) {
	if a == nil {
		return nil, nil
	}
	var args = make([]ast.FunctionArgument, len(a.Rest)+1)
	firstArg, err := a.First.CompileToFunctionArgument()
	if err != nil {
		return nil, err
	}
	args[0] = firstArg
	for i, t := range a.Rest {
		arg, err := t.FunctionArgument.CompileToFunctionArgument()
		if err != nil {
			return nil, err
		}
		args[i+1] = arg
	}
	return args, nil
}

type FunctionExprArgumentsRest struct {
	grammar.Seq
	Comma Token `tok:"op,,"`
	FunctionArgument
}

type FunctionArgument struct {
	grammar.OneOf
	*Literal
	*FilterQuery
	*LogicalExpr
	*FunctionExpr
}

func (a *FunctionArgument) CompileToFunctionArgument() (ast.FunctionArgument, error) {
	switch {
	case a.Literal != nil:
		return a.Literal.CompileToLiteral()
	case a.FilterQuery != nil:
		return a.FilterQuery.CompileToQuery()
	case a.LogicalExpr != nil:
		logicalExpr, err := a.LogicalExpr.CompileToLogicalExpr()
		if err != nil {
			return nil, err
		}
		return ast.LogicalExprArgument{LogicalExpr: logicalExpr}, nil
	case a.FunctionExpr != nil:
		return a.FunctionExpr.CompileToFunctionExpr()
	default:
		panic("invalid FunctionArgument")
	}
}

type Segment struct {
	grammar.OneOf
	*ChildSegment
	*DescendantSegment
}

func (s *Segment) CompileToSegment() (ast.Segment, error) {
	switch {
	case s.ChildSegment != nil:
		return s.ChildSegment.CompileToSegment()
	case s.DescendantSegment != nil:
		return s.DescendantSegment.CompileToSegment()
	default:
		panic("invalid Segment")
	}
}

type ChildSegment struct {
	grammar.OneOf
	*BracketedSelection
	*DotSelection
}

func (s *ChildSegment) CompileToSegment() (ast.Segment, error) {
	var selectors []ast.Selector
	var err error
	switch {
	case s.BracketedSelection != nil:
		selectors, err = s.BracketedSelection.CompileToSelectors()
	case s.DotSelection != nil:
		selectors = s.DotSelection.CompileToSelectors()
	default:
		panic("invalid ChildSegment")
	}
	if err != nil {
		return ast.Segment{}, err
	}
	return ast.Segment{
		Type:      ast.ChildSegmentType,
		Selectors: selectors,
	}, nil
}

type DescendantSegment struct {
	grammar.OneOf
	*DescendantBracketedSelection
	DescendantWildcardSelector    *Token `tok:"op,..*"`
	DescendantMemberNameShorthand *Token `tok:"descendantmembernameshorthand"`
}

func (s *DescendantSegment) CompileToSegment() (ast.Segment, error) {
	var selectors []ast.Selector
	var err error
	switch {
	case s.DescendantBracketedSelection != nil:
		selectors, err = s.DescendantBracketedSelection.CompileToSelectors()
	case s.DescendantWildcardSelector != nil:
		selectors = []ast.Selector{ast.WildcardSelector{}}
	case s.DescendantMemberNameShorthand != nil:
		// The token is of the form '..name'
		selectors = []ast.Selector{ast.NameSelector{Name: s.DescendantMemberNameShorthand.TokValue[2:]}}
	default:
		panic("invalid DescendantSegment")
	}
	if err != nil {
		return ast.Segment{}, nil
	}
	return ast.Segment{
		Type:      ast.DescendantSegmentType,
		Selectors: selectors,
	}, nil
}

type BracketedSelection struct {
	grammar.Seq
	OpenSquareBracket  Token `tok:"op,["`
	FirstSelector      Selector
	SelectorRest       []SelectorRest
	CloseSquareBracket Token `tok:"op,]"`
}

func (s *BracketedSelection) CompileToSelectors() ([]ast.Selector, error) {
	var selectors = make([]ast.Selector, len(s.SelectorRest)+1)
	firstSelector, err := s.FirstSelector.CompileToSelector()
	if err != nil {
		return nil, fmt.Errorf("first selector invalid: %w", err)
	}
	selectors[0] = firstSelector
	for i, sel := range s.SelectorRest {
		selector, err := sel.CompileToSelector()
		if err != nil {
			return nil, fmt.Errorf("selector %d invalid: %w", i+2, err)
		}
		selectors[i+1] = selector
	}
	return selectors, nil
}

type DescendantBracketedSelection struct {
	grammar.Seq
	OpenSquareBracket  Token `tok:"op,..["`
	FirstSelector      Selector
	SelectorRest       []SelectorRest
	CloseSquareBracket Token `tok:"op,]"`
}

func (s *DescendantBracketedSelection) CompileToSelectors() ([]ast.Selector, error) {
	var selectors = make([]ast.Selector, len(s.SelectorRest)+1)
	firstSelector, err := s.FirstSelector.CompileToSelector()
	if err != nil {
		return nil, fmt.Errorf("first selector invalid: %w", err)
	}
	selectors[0] = firstSelector
	for i, sel := range s.SelectorRest {
		selector, err := sel.CompileToSelector()
		if err != nil {
			return nil, fmt.Errorf("selector %d invalid: %w", i+2, err)
		}
		selectors[i+1] = selector
	}
	return selectors, nil
}

type SelectorRest struct {
	grammar.Seq
	Comma Token `tok:"op,,"`
	Selector
}

type DotSelection struct {
	grammar.OneOf
	WildcardSelector    *Token `tok:"op,.*"`
	MemberNameShorthand *Token `tok:"membernameshorthand"`
}

func (s *DotSelection) CompileToSelectors() []ast.Selector {
	var selector ast.Selector
	switch {
	case s.WildcardSelector != nil:
		selector = ast.WildcardSelector{}
	case s.MemberNameShorthand != nil:
		// s.MemberNameShorthand is of the form '.name'
		selector = ast.NameSelector{Name: s.MemberNameShorthand.TokValue[1:]}
	default:
		panic("invalid DotSelection")
	}
	return []ast.Selector{selector}
}
