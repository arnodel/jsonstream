package jsonpathtransformer

import (
	"github.com/arnodel/jsonstream/iterator"
	"github.com/arnodel/jsonstream/token"
)

type MainQueryRunner struct {
	mainRunner           QueryEvaluator
	innerSingularQueries []SingularQueryRunner
	innerQueries         []QueryEvaluator
	strictMode           bool
}

func (r MainQueryRunner) Transform(in <-chan token.Token, out token.WriteStream) {
	next := streamWritingProcessor{out: out}
	iter := iterator.New(token.ChannelReadStream(in))
	for iter.Advance() {
		value := iter.CurrentValue()
		r.mainRunner.MapValue(r.computeRunContext(value), value, next)
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
	return valueMapperNodesResult{
		ValueMapper: r.mainRunner,
		ctx:         r.computeRunContext(value),
		value:       value,
	}
}

type RunContext struct {
	innerSingularQueries []iterator.Value
	innerQueries         []*iterator.Iterator
	strictMode           bool
}

type ValueMapper interface {
	MapValue(ctx *RunContext, value iterator.Value, next valueProcessor) bool
}

type QueryEvaluator struct {
	ValueMapper
}

func (e QueryEvaluator) TransformValue(ctx *RunContext, value iterator.Value, out token.WriteStream) {
	e.MapValue(ctx, value, streamWritingProcessor{out: out})
}

func (e QueryEvaluator) EvaluateTruth(ctx *RunContext, value iterator.Value) bool {
	var detach func()
	value, detach = value.Clone()
	if detach != nil {
		defer detach()
	}
	return !e.MapValue(ctx, value, haltingProcessor{})
}

func (e QueryEvaluator) EvaluateNodesResult(ctx *RunContext, value iterator.Value) NodesResult {
	return valueMapperNodesResult{
		ValueMapper: e,
		ctx:         ctx,
		value:       value,
	}
}

type QueryRunner struct {
	isRootNodeQuery bool
	segments        []SegmentRunner
}

func (r QueryRunner) MapValue(ctx *RunContext, value iterator.Value, next valueProcessor) bool {
	if len(r.segments) == 0 {
		return next.ProcessValue(ctx, value)
	}
	return r.segments[0].transformValue(ctx, value, next, r.segments[1:])
}

type NodesResultEvaluator interface {
	EvaluateNodesResult(ctx *RunContext, value iterator.Value) NodesResult
}

type NodesResult interface {
	ForEachNode(func(iterator.Value) bool)
}

type valueMapperNodesResult struct {
	ValueMapper
	ctx   *RunContext
	value iterator.Value
}

func (r valueMapperNodesResult) ForEachNode(p func(iterator.Value) bool) {
	r.MapValue(r.ctx, r.value, callbackProcessor(p))
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

func (r InnerQueryRunner) MapValue(ctx *RunContext, value iterator.Value, next valueProcessor) bool {
	iter, detach := ctx.innerQueries[r.index].Clone()
	defer detach()
	for iter.Advance() {
		if !next.ProcessValue(ctx, iter.CurrentValue()) {
			return false
		}
	}
	return true
}

type InnerSingularQueryRunner struct {
	index int
}

func (r InnerSingularQueryRunner) Evaluate(ctx *RunContext, value iterator.Value) iterator.Value {
	return ctx.innerSingularQueries[r.index]
}
