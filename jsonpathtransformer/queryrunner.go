package jsonpathtransformer

import (
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

type MainQueryRunner struct {
	mainRunner           QueryRunner
	innerSingularQueries []SingularQueryRunner
	innerQueries         []QueryRunner
}

func (r MainQueryRunner) Transform(in <-chan token.Token, out token.WriteStream) {
	next := streamWritingProcessor{out: out}
	iter := iterator.New(token.ChannelReadStream(in))
	for iter.Advance() {
		value := iter.CurrentValue()
		r.mainRunner.transformValue(r.computeRunContext(value), value, next)
	}
}

func (r MainQueryRunner) computeRunContext(value iterator.Value) *RunContext {
	ctx := &RunContext{
		innerSingularQueries: make([]iterator.Value, len(r.innerSingularQueries)),
		innerQueries:         make([]*iterator.Iterator, len(r.innerQueries)),
	}
	for i, q := range r.innerSingularQueries {
		clone, detach := value.Clone()

		// There is nothing useful in the context to compute a singular query
		// value, so it is safe to pass nil.
		val := q.Evaluate(nil, clone)
		if scalar, ok := val.(*iterator.Scalar); ok {
			ctx.innerSingularQueries[i] = scalar
		} else {
			dest := token.NewAccumulatorStream()
			val.Copy(dest)
			cursor := token.NewCursorFromData(dest.GetTokens())
			iter := iterator.New(cursor)
			iter.Advance()
			ctx.innerSingularQueries[i] = iter.CurrentValue()
		}
		detach()
	}
	for i, q := range r.innerQueries {
		clone, detach := value.Clone()
		dest := token.NewAccumulatorStream()

		// The only inner queries that q may use are singular (already computed)
		// or they have a lower index in r.innerQueries than q so are also
		// already computed.  This makes the following call safe.
		q.TransformValue(ctx, clone, dest)
		cursor := token.NewCursorFromData(dest.GetTokens())
		ctx.innerQueries[i] = iterator.New(cursor)
		detach()
	}
	return ctx
}

func (r MainQueryRunner) TransformValue(value iterator.Value, out token.WriteStream) {
	r.mainRunner.TransformValue(r.computeRunContext(value), value, out)
}

func (r MainQueryRunner) EvaluateNodesResult(value iterator.Value) NodesResult {
	return queryRunnerNodesResult{
		QueryRunner: r.mainRunner,
		ctx:         r.computeRunContext(value),
		value:       value,
	}
}

type RunContext struct {
	innerSingularQueries []iterator.Value
	innerQueries         []*iterator.Iterator
}

type QueryRunner struct {
	isRootNodeQuery bool
	segments        []SegmentRunner
}

func (r QueryRunner) TransformValue(ctx *RunContext, value iterator.Value, out token.WriteStream) {
	r.transformValue(ctx, value, streamWritingProcessor{out: out})
}

func (r QueryRunner) transformValue(ctx *RunContext, value iterator.Value, next valueProcessor) bool {
	if len(r.segments) == 0 {
		return next.ProcessValue(ctx, value)
	}
	return r.segments[0].transformValue2(ctx, value, next, r.segments[1:])
}

func (r QueryRunner) EvaluateTruth(ctx *RunContext, value iterator.Value) bool {
	var detach func()
	value, detach = value.Clone()
	if detach != nil {
		defer detach()
	}
	return !r.transformValue(ctx, value, haltingProcessor{})
}

func (r QueryRunner) EvaluateNodesResult(ctx *RunContext, value iterator.Value) NodesResult {
	return queryRunnerNodesResult{
		QueryRunner: r,
		ctx:         ctx,
		value:       value,
	}
}

type NodesResultEvaluator interface {
	EvaluateNodesResult(ctx *RunContext, value iterator.Value) NodesResult
}

type NodesResult interface {
	ForEachNode(func(iterator.Value) bool)
}

type queryRunnerNodesResult struct {
	QueryRunner
	ctx   *RunContext
	value iterator.Value
}

func (r queryRunnerNodesResult) ForEachNode(p func(iterator.Value) bool) {
	r.transformValue(r.ctx, r.value, callbackProcessor(p))
}

type valueProcessor interface {
	// ProcessValue processes the value and returns true if the caller should
	// continue, false if the caller can stop.
	ProcessValue(ctx *RunContext, value iterator.Value) bool
}

type callbackProcessor func(iterator.Value) bool

func (p callbackProcessor) ProcessValue(ctx *RunContext, value iterator.Value) bool {
	return p(value)
}

type streamWritingProcessor struct {
	out token.WriteStream
}

func (a streamWritingProcessor) ProcessValue(ctx *RunContext, value iterator.Value) bool {
	value.Copy(a.out)
	return true
}

type haltingProcessor struct{}

func (p haltingProcessor) ProcessValue(ctx *RunContext, value iterator.Value) bool {
	return false
}

type InnerQueryRunner struct {
	index int
}

func (r InnerQueryRunner) EvaluateTruth(ctx *RunContext, value iterator.Value) bool {
	iter, detach := ctx.innerQueries[r.index].Clone()
	defer detach()
	return iter.Advance()
}

type InnerSingularQueryRunner struct {
	index int
}

func (r InnerSingularQueryRunner) Evaluate(ctx *RunContext, value iterator.Value) iterator.Value {
	return ctx.innerSingularQueries[r.index]
}
