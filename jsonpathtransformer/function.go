package jsonpathtransformer

import (
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
