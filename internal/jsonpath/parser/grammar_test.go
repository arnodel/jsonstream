package parser

import (
	"os"
	"reflect"
	"testing"

	"github.com/arnodel/grammar"
	"github.com/arnodel/jsonstream/internal/jsonpath/ast"
)

func TestGrammar(t *testing.T) {
	var tests = []struct {
		name  string
		input string
		query any
		ast   any
		err   *grammar.ParseError
	}{
		{
			name:  "example 1",
			input: "$.store.book[*].author",
			query: Query{
				RootIdentifier: tok("op", "$"),
				Segments: []Segment{
					{
						ChildSegment: &ChildSegment{
							DotSelection: &DotSelection{
								MemberNameShorthand: tokp("membernameshorthand", ".store"),
							},
						},
					},
					{
						ChildSegment: &ChildSegment{
							DotSelection: &DotSelection{
								MemberNameShorthand: tokp("membernameshorthand", ".book"),
							},
						},
					},
					{
						ChildSegment: &ChildSegment{
							BracketedSelection: &BracketedSelection{
								OpenSquareBracket: tok("op", "["),
								FirstSelector: Selector{
									WildcardSelector: tokp("op", "*"),
								},
								CloseSquareBracket: tok("op", "]"),
							},
						},
					},
					{
						ChildSegment: &ChildSegment{
							DotSelection: &DotSelection{
								MemberNameShorthand: tokp("membernameshorthand", ".author"),
							},
						},
					},
				},
			},
			ast: ast.Query{
				RootNode: ast.RootNodeIdentifier,
				Segments: []ast.Segment{
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "store"},
						},
					},
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "book"},
						},
					},
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.WildcardSelector{},
						},
					},
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "author"},
						},
					},
				},
			},
		},
		{
			name:  "example 2",
			input: "$..author",
			query: Query{
				RootIdentifier: tok("op", "$"),
				Segments: []Segment{
					{
						DescendantSegment: &DescendantSegment{
							DescendantMemberNameShorthand: tokp("descendantmembernameshorthand", "..author"),
						},
					},
				},
			},
			ast: ast.Query{
				RootNode: ast.RootNodeIdentifier,
				Segments: []ast.Segment{
					{
						Type: ast.DescendantSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "author"},
						},
					},
				},
			},
		},
		{
			name:  "example 3",
			input: "$.store.*",
			query: Query{
				RootIdentifier: tok("op", "$"),
				Segments: []Segment{
					{
						ChildSegment: &ChildSegment{
							DotSelection: &DotSelection{
								MemberNameShorthand: tokp("membernameshorthand", ".store"),
							},
						},
					},
					{
						ChildSegment: &ChildSegment{
							DotSelection: &DotSelection{
								WildcardSelector: tokp("op", ".*"),
							},
						},
					},
				},
			},
			ast: ast.Query{
				RootNode: ast.RootNodeIdentifier,
				Segments: []ast.Segment{
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "store"},
						},
					},
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.WildcardSelector{},
						},
					},
				},
			},
		},
		{
			name:  "example 4",
			input: "$.store..price",
			query: Query{
				RootIdentifier: tok("op", "$"),
				Segments: []Segment{
					{
						ChildSegment: &ChildSegment{
							DotSelection: &DotSelection{
								MemberNameShorthand: tokp("membernameshorthand", ".store"),
							},
						},
					},
					{
						DescendantSegment: &DescendantSegment{
							DescendantMemberNameShorthand: tokp("descendantmembernameshorthand", "..price"),
						},
					},
				},
			},
			ast: ast.Query{
				RootNode: ast.RootNodeIdentifier,
				Segments: []ast.Segment{
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "store"},
						},
					},
					{
						Type: ast.DescendantSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "price"},
						},
					},
				},
			},
		},
		{
			name:  "example 5",
			input: "$..book[2]",
			query: Query{
				RootIdentifier: tok("op", "$"),
				Segments: []Segment{
					{
						DescendantSegment: &DescendantSegment{
							DescendantMemberNameShorthand: tokp("descendantmembernameshorthand", "..book"),
						},
					},
					{
						ChildSegment: &ChildSegment{
							BracketedSelection: &BracketedSelection{
								OpenSquareBracket: tok("op", "["),
								FirstSelector: Selector{
									IndexSelector: tokp("int", "2"),
								},
								CloseSquareBracket: tok("op", "]"),
							},
						},
					},
				},
			},
			ast: ast.Query{
				RootNode: ast.RootNodeIdentifier,
				Segments: []ast.Segment{
					{
						Type: ast.DescendantSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "book"},
						},
					},
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.IndexSelector{Index: 2},
						},
					},
				},
			},
		},
		{
			name:  "example 9a",
			input: "$..book[0,1]",
			query: Query{
				RootIdentifier: tok("op", "$"),
				Segments: []Segment{
					{
						DescendantSegment: &DescendantSegment{
							DescendantMemberNameShorthand: tokp("descendantmembernameshorthand", "..book"),
						},
					},
					{
						ChildSegment: &ChildSegment{
							BracketedSelection: &BracketedSelection{
								OpenSquareBracket: tok("op", "["),
								FirstSelector: Selector{
									IndexSelector: tokp("int", "0"),
								},
								SelectorRest: []SelectorRest{
									{
										Comma: tok("op", ","),
										Selector: Selector{
											IndexSelector: tokp("int", "1"),
										},
									},
								},
								CloseSquareBracket: tok("op", "]"),
							},
						},
					},
				},
			},
			ast: ast.Query{
				RootNode: ast.RootNodeIdentifier,
				Segments: []ast.Segment{
					{
						Type: ast.DescendantSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "book"},
						},
					},
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.IndexSelector{Index: 0},
							ast.IndexSelector{Index: 1},
						},
					},
				},
			},
		},
		{
			name:  "example 9b",
			input: "$..book[:2]",
			query: Query{
				RootIdentifier: tok("op", "$"),
				Segments: []Segment{
					{
						DescendantSegment: &DescendantSegment{
							DescendantMemberNameShorthand: tokp("descendantmembernameshorthand", "..book"),
						},
					},
					{
						ChildSegment: &ChildSegment{
							BracketedSelection: &BracketedSelection{
								OpenSquareBracket: tok("op", "["),
								FirstSelector: Selector{
									SliceSelector: &SliceSelector{
										Colon: tok("op", ":"),
										End:   tokp("int", "2"),
									},
								},
								CloseSquareBracket: tok("op", "]"),
							},
						},
					},
				},
			},
			ast: ast.Query{
				RootNode: ast.RootNodeIdentifier,
				Segments: []ast.Segment{
					{
						Type: ast.DescendantSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "book"},
						},
					},
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.SliceSelector{End: pint64(2), Step: 1},
						},
					},
				},
			},
		},
		{
			name:  "example 10",
			input: "$..book[?@.isbn]",
			query: Query{
				RootIdentifier: tok("op", "$"),
				Segments: []Segment{
					{
						DescendantSegment: &DescendantSegment{
							DescendantMemberNameShorthand: tokp("descendantmembernameshorthand", "..book"),
						},
					},
					{
						ChildSegment: &ChildSegment{
							BracketedSelection: &BracketedSelection{
								OpenSquareBracket: tok("op", "["),
								FirstSelector: Selector{
									FilterSelector: &FilterSelector{
										FilterIdentifier: tok("op", "?"),
										LogicalExpr: LogicalExpr{
											First: LogicalAndExpr{
												First: BasicExpr{
													TestExpr: &TestExpr{
														BasicTestExpr: BasicTestExpr{
															FilterQuery: &FilterQuery{
																RelQuery: &RelQuery{
																	CurrentNodeIdentifier: tok("op", "@"),
																	Segments: []Segment{
																		{
																			ChildSegment: &ChildSegment{
																				DotSelection: &DotSelection{
																					MemberNameShorthand: tokp("membernameshorthand", ".isbn"),
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
								CloseSquareBracket: tok("op", "]"),
							},
						},
					},
				},
			},
			ast: ast.Query{
				RootNode: ast.RootNodeIdentifier,
				Segments: []ast.Segment{
					{
						Type: ast.DescendantSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "book"},
						},
					},
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.FilterSelector{
								Condition: ast.Query{
									RootNode: ast.CurrentNodeIdentifier,
									Segments: []ast.Segment{
										{
											Type: ast.ChildSegmentType,
											Selectors: []ast.Selector{
												ast.NameSelector{Name: "isbn"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:  "example 11",
			input: "$..book[?@.price < 10]",
			query: Query{
				RootIdentifier: tok("op", "$"),
				Segments: []Segment{
					{
						DescendantSegment: &DescendantSegment{
							DescendantMemberNameShorthand: tokp("descendantmembernameshorthand", "..book"),
						},
					},
					{
						ChildSegment: &ChildSegment{
							BracketedSelection: &BracketedSelection{
								OpenSquareBracket: tok("op", "["),
								FirstSelector: Selector{
									FilterSelector: &FilterSelector{
										FilterIdentifier: tok("op", "?"),
										LogicalExpr: LogicalExpr{
											First: LogicalAndExpr{
												First: BasicExpr{
													ComparisonExpr: &ComparisonExpr{
														Left: Comparable{
															SingularQuery: &SingularQuery{
																RelSingularQuery: &RelSingularQuery{
																	CurrentNodeIdentifier: tok("op", "@"),
																	Segments: []SingularQuerySegment{
																		{
																			DotNameSegment: tokp("membernameshorthand", ".price"),
																		},
																	},
																},
															},
														},
														Op: tok("comparisonop", "<"),
														Right: Comparable{
															Literal: &Literal{
																Number: tokp("int", "10"),
															},
														},
													},
												},
											},
										},
									},
								},
								CloseSquareBracket: tok("op", "]"),
							},
						},
					},
				},
			},
			ast: ast.Query{
				RootNode: ast.RootNodeIdentifier,
				Segments: []ast.Segment{
					{
						Type: ast.DescendantSegmentType,
						Selectors: []ast.Selector{
							ast.NameSelector{Name: "book"},
						},
					},
					{
						Type: ast.ChildSegmentType,
						Selectors: []ast.Selector{
							ast.FilterSelector{
								Condition: ast.ComparisonExpr{
									Left: ast.SingularQuery{
										RootNode: ast.CurrentNodeIdentifier,
										Segments: []ast.SingularQuerySegment{
											ast.NameSegment{Name: "price"},
										},
									},
									Op:    ast.LessThanOp,
									Right: ast.Literal{Value: float64(10)},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream, err := TokeniseJsonPathString(test.input)
			if err != nil {
				t.Fatalf("unexpected error tokenising %s: %s", test.name, err)
			}
			var query Query
			err = grammar.Parse(&query, stream)
			if err != test.err {
				t.Fatalf("exprected error to be %q, got %q", test.err, err)
			}
			if !reflect.DeepEqual(query, test.query) {
				grammar.PrettyWrite(os.Stdout, query)
				grammar.PrettyWrite(os.Stdout, test.query)
				t.Fatalf("query error")
			}
			if test.ast != nil {
				astQuery := query.CompileToQuery()
				if !reflect.DeepEqual(astQuery, test.ast) {
					t.Fatalf("ast query error")
				}
			}
		})
	}
}

func pint64(n int64) *int64 {
	return &n
}
