package jsonpath

import (
	"regexp"
	"unicode/utf8"

	"github.com/arnodel/jsonstream/internal/jsonpath/iregexp"
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

type FunctionDef struct {
	Name       string
	InputTypes []Type
	OutputType Type
	Run        func([]any) any
}

type Type uint8

const (
	ValueType Type = iota
	LogicalType
	NodesType
)

func (t Type) ConvertsTo(u Type) bool {
	return t == u || t == NodesType && u == LogicalType
}

func (t Type) String() string {
	switch t {
	case ValueType:
		return "value"
	case LogicalType:
		return "logical"
	case NodesType:
		return "nodes"
	default:
		panic("invalid type")
	}
}

type FunctionRegistry interface {
	GetFunctionDef(name string) *FunctionDef
	AddFunctionDef(def FunctionDef) error
}

func NewFunctionRegistry() FunctionRegistry {
	return functionDefMap{}
}

type functionDefMap map[string]*FunctionDef

func (m functionDefMap) AddFunctionDef(def FunctionDef) error {
	m[def.Name] = &def
	return nil
}

func (m functionDefMap) GetFunctionDef(name string) *FunctionDef {
	return m[name]
}

// This contains the functions defined in the jsonpath spec
var DefaultFunctionRegistry = NewFunctionRegistry()

func init() {
	DefaultFunctionRegistry.AddFunctionDef(FunctionDef{
		Name:       "length",
		InputTypes: []Type{ValueType},
		OutputType: ValueType,
		Run:        run_length,
	})
	DefaultFunctionRegistry.AddFunctionDef(FunctionDef{
		Name:       "count",
		InputTypes: []Type{NodesType},
		OutputType: ValueType,
		Run:        run_count,
	})
	DefaultFunctionRegistry.AddFunctionDef(FunctionDef{
		Name:       "match",
		InputTypes: []Type{ValueType, ValueType},
		OutputType: LogicalType,
		Run:        run_match,
	})
	DefaultFunctionRegistry.AddFunctionDef(FunctionDef{
		Name:       "search",
		InputTypes: []Type{ValueType, ValueType},
		OutputType: LogicalType,
		Run:        run_search,
	})
	DefaultFunctionRegistry.AddFunctionDef(FunctionDef{
		Name:       "value",
		InputTypes: []Type{NodesType},
		OutputType: ValueType,
		Run:        run_value,
	})
}

func run_length(args []any) any {
	var n int64
	switch x := args[0].(type) {
	case *iterator.Array:
		for x.Advance() {
			n++
		}
	case *iterator.Object:
		var n int64
		for x.Advance() {
			n++
		}
	case *iterator.Scalar:
		scalar := x.Scalar()
		if scalar.Type() != token.String {
			return nil
		}
		n = int64(utf8.RuneCountInString(scalar.ToString()))
	default:
		return nil
	}
	return (*iterator.Scalar)(token.Int64Scalar(n))
}

func run_count(args []any) any {
	var n int64
	args[0].(NodesResult).ForEachNode(func(value iterator.Value) bool {
		n++
		return true
	})
	return (*iterator.Scalar)(token.Int64Scalar(n))
}

func run_match(args []any) any {
	return regexMatches(args, true)
}

func run_search(args []any) any {
	return regexMatches(args, false)
}

// regexMatches implements match() (fullMatch) and search() for arguments
// evaluated at run time.  When the pattern is a literal the compiler uses a
// regexFunctionRunner instead, which compiles it once.
func regexMatches(args []any, fullMatch bool) bool {
	arg1, ok := args[0].(*iterator.Scalar)
	if !ok {
		return false
	}
	arg2, ok := args[1].(*iterator.Scalar)
	if !ok {
		return false
	}
	if arg1.Scalar().Type() != token.String || arg2.Scalar().Type() != token.String {
		return false
	}
	var re *regexp.Regexp
	var err error
	if fullMatch {
		re, err = iregexp.CompileAnchored(arg2.Scalar().ToString())
	} else {
		re, err = iregexp.Compile(arg2.Scalar().ToString())
	}
	if err != nil {
		return false
	}
	if arg1.Scalar().IsUnescaped() {
		return re.Match(arg1.Bytes[1 : len(arg1.Bytes)-1])
	}
	return re.MatchString(arg1.Scalar().ToString())
}

func run_value(args []any) any {
	var singleValue iterator.Value
	args[0].(NodesResult).ForEachNode(func(v iterator.Value) bool {
		if singleValue != nil {
			singleValue = nil
			return false
		}
		singleValue = v
		return true
	})
	return singleValue
}
