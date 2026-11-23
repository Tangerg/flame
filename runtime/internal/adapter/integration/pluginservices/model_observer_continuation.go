package pluginservices

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"iter"
	"sync"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	coreinvocation "github.com/Tangerg/scope/core/invocation"
)

type continuationState uint8

const (
	awaitingContinuation continuationState = iota
	runningContinuation
	settledContinuation
)

type observedChunk struct {
	delta        *chat.ResponseDelta
	wire         framevalue.JSON
	acknowledged chan struct{}
}

type observedContinuation struct {
	mu        sync.Mutex
	request   framevalue.JSON
	id        pluginmcp.CommandID
	state     continuationState
	response  *chat.Response
	failure   error
	violation error
	pending   *observedChunk
}

func (a *observedContinuation) enter(command pluginmcp.Command[scopewire.Request]) error {
	input, err := framevalue.EncodeJSON(command.Input)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != awaitingContinuation || command.ID != a.id || err != nil || input != a.request {
		a.violation = errors.Join(err, &framecall.Refusal{Code: refusalObserverContract, Message: "transparent observer cannot repeat or change its continuation"})
		return a.violation
	}
	a.state = runningContinuation
	return nil
}

func (a *observedContinuation) finish(response *chat.Response, failure error) {
	if response != nil {
		if err := response.Validate(); err != nil {
			failure = errors.Join(failure, err)
			response = nil
		}
	}
	if response == nil && failure == nil {
		failure = errors.New("model observer continuation returned no response")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.response = response.Clone()
	a.failure = failure
	a.state = settledContinuation
}

func (a *observedContinuation) outcome(observerErr error) (*chat.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	observerErr = errors.Join(observerErr, a.violation)
	switch a.state {
	case awaitingContinuation:
		if observerErr == nil {
			observerErr = errors.New("model observer omitted its captured continuation")
		}
		return nil, notExecuted(observerErr)
	case settledContinuation:
		response := a.response.Clone()
		if observerErr == nil {
			return response, a.failure
		}
		kind := coreinvocation.ClassifyFailure(a.failure)
		if kind == coreinvocation.FailureNone {
			kind = coreinvocation.FailureFailed
		}
		return response, &coreinvocation.Failure{Kind: kind, Cause: errors.Join(a.failure, observerErr)}
	default:
		return nil, errors.Join(errors.New("model observer continuation did not settle"), observerErr)
	}
}

func notExecuted(cause error) error {
	return &coreinvocation.Failure{Kind: coreinvocation.FailureNotExecuted, Cause: cause}
}

func (a *observedContinuation) relay(ctx context.Context, stream iter.Seq2[*chat.ResponseDelta, error], yield func(*chat.ResponseDelta, error) bool) {
	var accumulated chat.ResponseAccumulator
	failure := pluginmcp.ErrStreamUnconfirmed
	defer func() {
		response, err := accumulated.Response()
		if failure == nil {
			failure = err
		}
		a.finish(response, failure)
	}()
	if stream == nil {
		failure = chat.ErrNilStream
		yield(nil, failure)
		return
	}
	for delta, err := range stream {
		if err == nil {
			err = accumulated.Add(delta)
		}
		if err != nil {
			failure = err
			yield(nil, err)
			return
		}
		chunk, err := a.captureChunk(delta)
		if err != nil {
			failure = err
			yield(nil, err)
			return
		}
		if !yield(chunk.delta.Clone(), nil) {
			return
		}
		select {
		case <-chunk.acknowledged:
		case <-ctx.Done():
			failure = errors.Join(failure, context.Cause(ctx))
			return
		}
	}
	failure = nil
}

func (a *observedContinuation) captureChunk(delta *chat.ResponseDelta) (*observedChunk, error) {
	frozen := delta.Clone()
	wire, err := framevalue.EncodeJSON((*scopewire.ResponseDelta)(frozen))
	if err != nil {
		return nil, err
	}
	chunk := &observedChunk{delta: frozen, wire: wire, acknowledged: make(chan struct{})}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pending = chunk
	return chunk, nil
}

func (a *observedContinuation) matchChunk(reply []byte) (*observedChunk, error) {
	wire, err := framevalue.ParseJSON(reply)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending == nil || wire != a.pending.wire {
		return nil, errors.New("model observer changed or reordered its captured chunk")
	}
	return a.pending, nil
}

func (a *observedContinuation) acknowledgeChunk(chunk *observedChunk) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pending = nil
	close(chunk.acknowledged)
}

func (a *observedContinuation) forward(values iter.Seq2[pluginmcp.StreamValue[jsontext.Value, jsontext.Value], error], yield func(*chat.ResponseDelta, error) bool) (consumed bool, err error) {
	for value, err := range values {
		if err != nil {
			return true, err
		}
		encoded, isChunk := value.Chunk()
		if !isChunk {
			continue
		}
		chunk, err := a.matchChunk(encoded)
		if err != nil {
			return true, err
		}
		if !yield(chunk.delta.Clone(), nil) {
			return false, nil
		}
		a.acknowledgeChunk(chunk)
	}
	return true, nil
}
