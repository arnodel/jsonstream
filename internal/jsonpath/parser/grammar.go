// Package parser implements a parser for jsonpath.
//
// Grammar taken from https://datatracker.ietf.org/doc/draft-ietf-jsonpath-base/21/
package parser

import (
	"github.com/arnodel/grammar"
	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
)

type Token = grammar.SimpleToken

type Query struct {
	grammar.Seq
	RootIdentifier Token `tok:"op,$"`
	Segments       []Segment
}

func (q *Query) CompileToQuery() ast.Query {
	var compiledSegments = make([]ast.Segment, len(q.Segments))
	for i, s := range q.Segments {
		compiledSegments[i] = s.CompileToSegment()
	}
	return ast.Query{
		RootNode: ast.RootNodeIdentifier,
		Segments: compiledSegments,
	}
}

type Selector struct {
	grammar.OneOf
	NameSelector     *StringLiteral
	WildcardSelector *Token `tok:"op,*"`
	*SliceSelector
	IndexSelector *Token `tok:"int"`
	*FilterSelector
}

func (s *Selector) CompileToSelector() ast.Selector {
	switch {
	case s.NameSelector != nil:
		return ast.NameSelector{Name: s.NameSelector.CompileToString()}
	case s.WildcardSelector != nil:
		return ast.WildcardSelector{}
	case s.SliceSelector != nil:
		return s.SliceSelector.CompileToSelector()
	case s.IndexSelector != nil:
		return ast.IndexSelector{Index: parseInt(s.IndexSelector.TokValue)}
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

func (s *StringLiteral) CompileToString() string {
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

func (s *SliceSelector) CompileToSelector() ast.Selector {
	var start, end *int64
	var step int64 = 1
	if s.Start != nil {
		var startInt = parseInt(s.Start.TokValue)
		start = &startInt
	}
	if s.End != nil {
		var endInt = parseInt(s.End.TokValue)
		end = &endInt
	}
	if s.SliceStep != nil {
		step = parseInt(s.Step.TokValue)
	}
	return ast.SliceSelector{Start: start, End: end, Step: step}
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

func (s *FilterSelector) CompileToSelector() ast.Selector {
	return ast.FilterSelector{Condition: s.LogicalExpr.CompileToLogicalExpr()}
}

type LogicalExpr struct {
	grammar.Seq
	First LogicalAndExpr
	Rest  []LogicalOrRest
}

func (e *LogicalExpr) CompileToLogicalExpr() ast.LogicalExpr {
	var first = e.First.CompileToLogicalExpr()
	if len(e.Rest) == 0 {
		return first
	}
	var args = make([]ast.LogicalExpr, len(e.Rest)+1)
	args[0] = first
	for i, t := range e.Rest {
		args[i+1] = t.Operand.CompileToLogicalExpr()
	}
	return ast.OrExpr{Arguments: args}
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

func (e *LogicalAndExpr) CompileToLogicalExpr() ast.LogicalExpr {
	var first = e.First.CompileToLogicalExpr()
	if len(e.Rest) == 0 {
		return first
	}
	var args = make([]ast.LogicalExpr, len(e.Rest)+1)
	args[0] = first
	for i, t := range e.Rest {
		args[i+1] = t.Operand.CompileToLogicalExpr()
	}
	return ast.AndExpr{Arguments: args}
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

func (e *BasicExpr) CompileToLogicalExpr() ast.LogicalExpr {
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

func (e *ParenExpr) CompileToLogicalExpr() ast.LogicalExpr {
	var innerExpr = e.LogicalExpr.CompileToLogicalExpr()
	if e.Not != nil {
		return ast.NotExpr{Argument: innerExpr}
	}
	return innerExpr
}

type TestExpr struct {
	grammar.Seq
	Not *Token `tok:"op,!"`
	BasicTestExpr
}

func (e *TestExpr) CompileToLogicalExpr() ast.LogicalExpr {
	var innerExpr = e.BasicTestExpr.CompileToLogicalExpr()
	if e.Not != nil {
		return ast.NotExpr{Argument: innerExpr}
	}
	return innerExpr
}

type BasicTestExpr struct {
	grammar.OneOf
	*FilterQuery
	*FunctionExpr
}

func (e *BasicTestExpr) CompileToLogicalExpr() ast.LogicalExpr {
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

func (q *FilterQuery) CompileToQuery() ast.Query {
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

func (q *RelQuery) CompileToQuery() ast.Query {
	var compiledSegments = make([]ast.Segment, len(q.Segments))
	for i, s := range q.Segments {
		compiledSegments[i] = s.CompileToSegment()
	}
	return ast.Query{
		RootNode: ast.CurrentNodeIdentifier,
		Segments: compiledSegments,
	}
}

type ComparisonExpr struct {
	grammar.Seq
	Left  Comparable
	Op    Token `tok:"comparisonop"`
	Right Comparable
}

func (e *ComparisonExpr) CompileToLogicalExpr() ast.LogicalExpr {
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
	return ast.ComparisonExpr{
		Left:  e.Left.CompileToComparable(),
		Op:    op,
		Right: e.Right.CompileToComparable(),
	}
}

type Comparable struct {
	grammar.OneOf
	*Literal
	*SingularQuery
	*FunctionExpr
}

func (c *Comparable) CompileToComparable() ast.Comparable {
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

func (l *Literal) CompileToLiteral() ast.Literal {
	switch {
	case l.Number != nil:
		return ast.Literal{Value: parseNumber(l.Number.TokValue)}
	case l.StringLiteral != nil:
		return ast.Literal{Value: l.StringLiteral.CompileToString()}
	case l.Boolean != nil:
		switch l.Boolean.TokValue {
		case "true":
			return ast.Literal{Value: true}
		case "false":
			return ast.Literal{Value: false}
		default:
			panic("invalid Literal.Bool")
		}
	case l.Null != nil:
		return ast.Literal{}
	default:
		panic("invalid Literal")
	}
}

type SingularQuery struct {
	grammar.OneOf
	*RelSingularQuery
	*AbsSingularQuery
}

func (q *SingularQuery) CompileToSingularQuery() ast.SingularQuery {
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

func (q *RelSingularQuery) CompileToSingularQuery() ast.SingularQuery {
	var segments = make([]ast.SingularQuerySegment, len(q.Segments))
	for i, s := range q.Segments {
		segments[i] = s.CompileToSingularQuerySegment()
	}
	return ast.SingularQuery{
		RootNode: ast.CurrentNodeIdentifier,
		Segments: segments,
	}
}

type AbsSingularQuery struct {
	grammar.Seq
	RootIdentifier Token `tok:"op,$"`
	Segments       []SingularQuerySegment
}

func (q *AbsSingularQuery) CompileToSingularQuery() ast.SingularQuery {
	var segments = make([]ast.SingularQuerySegment, len(q.Segments))
	for i, s := range q.Segments {
		segments[i] = s.CompileToSingularQuerySegment()
	}
	return ast.SingularQuery{
		RootNode: ast.RootNodeIdentifier,
		Segments: segments,
	}
}

type SingularQuerySegment struct {
	grammar.OneOf
	*BracketNameSegment
	DotNameSegment *Token `tok:"membernameshorthand"`
	*IndexSegment
}

func (s *SingularQuerySegment) CompileToSingularQuerySegment() ast.SingularQuerySegment {
	switch {
	case s.BracketNameSegment != nil:
		return s.BracketNameSegment.CompileToSingularQuerySegment()
	case s.DotNameSegment != nil:
		// S.DotNameSegment is of the form '.name'
		return ast.NameSegment{Name: s.DotNameSegment.TokValue[1:]}
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

func (s *BracketNameSegment) CompileToSingularQuerySegment() ast.SingularQuerySegment {
	return ast.NameSegment{Name: s.NameSelector.CompileToString()}
}

type IndexSegment struct {
	grammar.Seq
	OpenSquareBracket  Token `tok:"op,["`
	IndexSelector      Token `tok:"int"`
	CloseSquareBracket Token `tok:"op,]"`
}

func (s *IndexSegment) CompileToSingularQuerySegment() ast.SingularQuerySegment {
	return ast.IndexSegment{Index: int(parseInt(s.IndexSelector.TokValue))}
}

type FunctionExpr struct {
	grammar.Seq
	FunctionName Token `tok:"functionname"`
	OpenBracket  Token `tok:"op,("`
	Arguments    *FunctionExprArguments
	CloseBracket Token `tok:"op,)"`
}

func (e *FunctionExpr) CompileToFunctionExpr() ast.FunctionExpr {
	return ast.FunctionExpr{
		FunctionName: e.FunctionName.TokValue,
		Arguments:    e.Arguments.CompileToFunctionArguments(),
	}
}

type FunctionExprArguments struct {
	grammar.Seq
	First FunctionArgument
	Rest  []FunctionExprArgumentsRest
}

func (a *FunctionExprArguments) CompileToFunctionArguments() []ast.FunctionArgument {
	if a == nil {
		return nil
	}
	var args = make([]ast.FunctionArgument, len(a.Rest)+1)
	args[0] = a.First.CompileToFunctionArgument()
	for i, t := range a.Rest {
		args[i+1] = t.FunctionArgument.CompileToFunctionArgument()
	}
	return args
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

func (a *FunctionArgument) CompileToFunctionArgument() ast.FunctionArgument {
	switch {
	case a.Literal != nil:
		return a.Literal.CompileToLiteral()
	case a.FilterQuery != nil:
		return a.FilterQuery.CompileToQuery()
	case a.LogicalExpr != nil:
		return ast.LogicalExprArgument{LogicalExpr: a.LogicalExpr.CompileToLogicalExpr()}
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

func (s *Segment) CompileToSegment() ast.Segment {
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

func (s *ChildSegment) CompileToSegment() ast.Segment {
	var selectors []ast.Selector
	switch {
	case s.BracketedSelection != nil:
		selectors = s.BracketedSelection.CompileToSelectors()
	case s.DotSelection != nil:
		selectors = s.DotSelection.CompileToSelectors()
	default:
		panic("invalid ChildSegment")
	}
	return ast.Segment{
		Type:      ast.ChildSegmentType,
		Selectors: selectors,
	}
}

type DescendantSegment struct {
	grammar.OneOf
	*DescendantBracketedSelection
	DescendantWildcardSelector    *Token `tok:"op,..*"`
	DescendantMemberNameShorthand *Token `tok:"descendantmembernameshorthand"`
}

func (s *DescendantSegment) CompileToSegment() ast.Segment {
	var selectors []ast.Selector
	switch {
	case s.DescendantBracketedSelection != nil:
		selectors = s.DescendantBracketedSelection.CompileToSelectors()
	case s.DescendantWildcardSelector != nil:
		selectors = []ast.Selector{ast.WildcardSelector{}}
	case s.DescendantMemberNameShorthand != nil:
		// The token is of the form '..name'
		selectors = []ast.Selector{ast.NameSelector{Name: s.DescendantMemberNameShorthand.TokValue[2:]}}
	default:
		panic("invalid DescendantSegment")
	}
	return ast.Segment{
		Type:      ast.DescendantSegmentType,
		Selectors: selectors,
	}
}

type BracketedSelection struct {
	grammar.Seq
	OpenSquareBracket  Token `tok:"op,["`
	FirstSelector      Selector
	SelectorRest       []SelectorRest
	CloseSquareBracket Token `tok:"op,]"`
}

func (s *BracketedSelection) CompileToSelectors() []ast.Selector {
	var selectors = make([]ast.Selector, len(s.SelectorRest)+1)
	selectors[0] = s.FirstSelector.CompileToSelector()
	for i, sel := range s.SelectorRest {
		selectors[i+1] = sel.CompileToSelector()
	}
	return selectors
}

type DescendantBracketedSelection struct {
	grammar.Seq
	OpenSquareBracket  Token `tok:"op,..["`
	FirstSelector      Selector
	SelectorRest       []SelectorRest
	CloseSquareBracket Token `tok:"op,]"`
}

func (s *DescendantBracketedSelection) CompileToSelectors() []ast.Selector {
	var selectors = make([]ast.Selector, len(s.SelectorRest)+1)
	selectors[0] = s.FirstSelector.CompileToSelector()
	for i, sel := range s.SelectorRest {
		selectors[i+1] = sel.CompileToSelector()
	}
	return selectors
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
