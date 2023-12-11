package jsonpathtransformer

import (
	"regexp"

	"github.com/arnodel/jsonstream/internal/jsonpath/parser"
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
}

func run_length(args []any) any {
	val := args[0].(iterator.Value)
	switch x := val.(type) {
	case *iterator.Array:
		var n int64
		for x.Advance() {
			n++
		}
		return (*iterator.Scalar)(token.Int64Scalar(n))
	case *iterator.Object:
		var n int64
		for x.Advance() {
			n++
		}
		return (*iterator.Scalar)(token.Int64Scalar(n))
	default:
		return nil
	}
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
	ptnString := parser.ParseJsonLiteralBytes(arg2.Bytes).(string)
	ptn, err := regexp.Compile(ptnString)
	if err != nil {
		return false
	}
	if arg1.Scalar().IsUnescaped() {
		loc := ptn.FindIndex(arg1.Bytes[1 : len(arg1.Bytes)-1])
		return loc != nil && loc[0] == 0 && loc[1] == len(arg1.Bytes)-2
	}
	arg1String := parser.ParseJsonLiteralBytes(arg1.Bytes).(string)
	loc := ptn.FindStringIndex(arg1String)
	return loc != nil && loc[0] == 0 && loc[1] == len(arg1String)
}
