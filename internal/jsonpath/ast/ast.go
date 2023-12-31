package ast

//
// Query
//

type NodeIdentifier uint8

const (
	RootNodeIdentifier NodeIdentifier = iota
	CurrentNodeIdentifier
)

type Query struct {
	RootNode NodeIdentifier
	Segments []Segment
}

func (q Query) AsSingularQuery() (sq SingularQuery, ok bool) {
	sq.RootNode = q.RootNode
	for _, s := range q.Segments {
		if s.Type != ChildSegmentType || len(s.Selectors) != 1 {
			return SingularQuery{}, false
		}
		sel := s.Selectors[0]
		switch x := sel.(type) {
		case NameSelector:
			sq.Segments = append(sq.Segments, NameSegment(x))
		case IndexSelector:
			sq.Segments = append(sq.Segments, IndexSegment(x))
		default:
			return SingularQuery{}, false
		}
	}
	return sq, true
}

type SegmentType uint8

const (
	ChildSegmentType SegmentType = iota
	DescendantSegmentType
)

type Segment struct {
	Type      SegmentType
	Selectors []Selector
}

//
// Selectors
//

type Selector interface{}

var _ Selector = NameSelector{}
var _ Selector = WildcardSelector{}
var _ Selector = IndexSelector{}
var _ Selector = FilterSelector{}
var _ Selector = SliceSelector{}

type NameSelector struct {
	Name string
}

type WildcardSelector struct{}

type IndexSelector struct {
	Index int64
}

type SliceSelector struct {
	Start, End *int64
	Step       int64
}

type FilterSelector struct {
	Condition LogicalExpr
}

//
// Logical expressions
//

type LogicalExpr interface{}

var _ LogicalExpr = OrExpr{}
var _ LogicalExpr = AndExpr{}
var _ LogicalExpr = NotExpr{}
var _ LogicalExpr = ComparisonExpr{}
var _ LogicalExpr = Query{}
var _ LogicalExpr = FunctionExpr{}

type OrExpr struct {
	Arguments []LogicalExpr
}

type AndExpr struct {
	Arguments []LogicalExpr
}

type NotExpr struct {
	Argument LogicalExpr
}

type ComparisonExpr struct {
	Left  Comparable
	Op    ComparisonOp
	Right Comparable
}

//
// Comparable
//

type Comparable interface{}

var _ Comparable = Literal{}
var _ Comparable = SingularQuery{}
var _ Comparable = FunctionExpr{}

type ComparisonOp uint8

const (
	EqualOp ComparisonOp = iota
	NotEqualOp
	LessThanOrEqualOp
	GreaterThanOrEqualOp
	LessThanOp
	GreaterThanOp
)

//
// Literals
//

type Literal struct {
	Value any // string, float64, bool, nil
}

//
// SingularQuery
//

type SingularQuery struct {
	RootNode NodeIdentifier
	Segments []SingularQuerySegment
}

type SingularQuerySegment interface{}

var _ SingularQuerySegment = NameSegment{}
var _ SingularQuerySegment = IndexSegment{}

type NameSegment struct {
	Name string
}

type IndexSegment struct {
	Index int64
}

//
// Function Expr
//

type FunctionExpr struct {
	FunctionName string
	Arguments    []FunctionArgument
}

type FunctionArgument interface{}

var _ FunctionArgument = Literal{}
var _ FunctionArgument = Query{}
var _ FunctionArgument = LogicalExprArgument{}
var _ FunctionArgument = FunctionExpr{}

type LogicalExprArgument struct {
	LogicalExpr
}
