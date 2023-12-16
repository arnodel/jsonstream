package jsonpathtransformer

import "github.com/arnodel/jsonstream/iterator"

type FunctionRunner struct {
	*FunctionDef
	argRunners []FunctionArgumentRunner
}

func (r FunctionRunner) run(ctx *RunContext, val iterator.Value) any {
	args := make([]any, len(r.argRunners))
	for i, argRunner := range r.argRunners {
		switch r.InputTypes[i] {
		case ValueType:
			clone, detach := val.Clone()
			args[i] = argRunner.Evaluate(ctx, clone)
			if detach != nil {
				defer detach()
			}
		case LogicalType:
			args[i] = argRunner.EvaluateTruth(ctx, val)
		case NodesType:
			args[i] = argRunner.EvaluateNodesResult(ctx, val)
		default:
			panic("invalid input type")
		}
	}
	return r.Run(args)
}

func (r FunctionRunner) EvaluateTruth(ctx *RunContext, val iterator.Value) bool {
	return r.run(ctx, val).(bool)
}

func (r FunctionRunner) Evaluate(ctx *RunContext, val iterator.Value) iterator.Value {
	result := r.run(ctx, val)
	if result == nil {
		return nil
	}
	return result.(iterator.Value)
}

func (r FunctionRunner) EvaluateNodesResult(ctx *RunContext, val iterator.Value) NodesResult {
	// I don't know how to do that yet, but also I don't know if this is allowed by the spec.
	panic("unimplemented")
}

type FunctionArgumentRunner interface {
	ComparableEvaluator
	LogicalEvaluator
	NodesResultEvaluator
}

type ValueArgumentRunner struct {
	ComparableEvaluator
}

func (r ValueArgumentRunner) EvaluateTruth(ctx *RunContext, val iterator.Value) bool {
	panic("invaid EvaluateTruth call")
}

func (r ValueArgumentRunner) EvaluateNodesResult(ctx *RunContext, val iterator.Value) NodesResult {
	panic("invalid EvaluateNodesResult call")
}

type LogicalArgumentRunner struct {
	LogicalEvaluator
}

func (r LogicalArgumentRunner) Evaluate(ctx *RunContext, val iterator.Value) iterator.Value {
	panic("invalid Evaluate call")
}

func (r LogicalArgumentRunner) EvaluateNodesResult(ctx *RunContext, val iterator.Value) NodesResult {
	panic("invalid EvaluateNodesResult call")
}

type NodesArgumentRunner struct {
	NodesResultEvaluator
}

func (r NodesArgumentRunner) EvaluateTruth(ctx *RunContext, val iterator.Value) bool {
	panic("invaid EvaluateTruth call")
}

func (r NodesArgumentRunner) Evaluate(ctx *RunContext, val iterator.Value) iterator.Value {
	panic("invalid Evaluate call")
}
