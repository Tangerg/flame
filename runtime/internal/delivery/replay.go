package delivery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/project"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	framecall "github.com/Tangerg/frame/call"

	"go.opentelemetry.io/otel/trace"
)

type storedOutcome struct {
	Value   jsontext.Value        `json:"value,omitzero"`
	Problem *protocol.ProblemData `json:"problem,omitzero"`
}

type keyedCall struct {
	replay      *operation.Replay
	method      *Method
	key         string
	fingerprint string
}

type ownedReplay func(target any, ctx context.Context, parameters any, call keyedCall) protocol.OperationResult

var errIdempotencyRebound = errors.New("idempotency key is already bound to another operation")

func (e *Endpoint) replayOperation(ctx context.Context, method *Method, parameters any, key string, execute func() protocol.OperationResult) protocol.OperationResult {
	fingerprint, err := operationFingerprint(method.Meta.Name, parameters)
	if err != nil {
		return failed(projectError(fmt.Errorf("idempotency: fingerprint operation: %w", err)))
	}
	call := keyedCall{replay: e.idempotency, method: method, key: key, fingerprint: fingerprint}
	if method.ownedReplay != nil {
		return method.ownedReplay(e.target, ctx, parameters, call)
	}
	var result protocol.OperationResult
	executed := false
	payload, err := e.idempotency.Invoke(ctx, key, fingerprint, func() ([]byte, error) {
		result = execute()
		executed = true
		return encodeStoredOutcome(result)
	})
	if executed {
		if err == nil {
			return result
		}
		if !errors.Is(err, operation.ErrOutcomePending) {
			call.reportUnconfirmed(ctx, err)
		}
	}
	return call.result(ctx, payload, err)
}

func (c keyedCall) result(ctx context.Context, payload []byte, err error) protocol.OperationResult {
	switch {
	case err == nil:
		return replayOutcome(c.method, payload)
	case errors.Is(err, framecall.ErrConflict):
		return failed(protocol.NewFailure(protocol.ErrIdempotencyConflict, errIdempotencyRebound.Error()))
	case errors.Is(err, operation.ErrOutcomePending):
		c.reportUnconfirmed(ctx, err)
		return failed(protocol.NewFailure(protocol.ErrIdempotencyInProgress, operation.ErrOutcomePending.Error()))
	case errors.Is(err, operation.ErrInProgress):
		return failed(protocol.NewFailure(protocol.ErrIdempotencyInProgress, operation.ErrInProgress.Error()))
	default:
		return failed(projectError(err))
	}
}

func (c keyedCall) reportUnconfirmed(ctx context.Context, err error) {
	slog.ErrorContext(ctx, "delivery: idempotency receipt is unconfirmed after a completed operation", telemetry.RPCMethod(c.method.Meta.Name.String()), telemetry.Error(err))
	trace.SpanFromContext(ctx).RecordError(fmt.Errorf("idempotency: store %s replay: %w", c.method.Meta.Name, err))
}

func (c keyedCall) invokeAtomic(ctx context.Context, commit func(atomicOutcome) error) protocol.OperationResult {
	payload, err := c.replay.InvokeAtomic(ctx, c.key, c.fingerprint, func(complete *operation.AtomicCompletion) error {
		return commit(atomicOutcome{method: c.method, complete: complete})
	})
	return c.result(ctx, payload, err)
}

func atomicReplay[Capability, Params any](commit func(Capability, context.Context, Params, atomicOutcome) error) func(Capability, context.Context, Params, keyedCall) protocol.OperationResult {
	return func(owner Capability, ctx context.Context, in Params, call keyedCall) protocol.OperationResult {
		return call.invokeAtomic(ctx, func(outcome atomicOutcome) error {
			return commit(owner, ctx, in, outcome)
		})
	}
}

type atomicOutcome struct {
	method   *Method
	complete *operation.AtomicCompletion
}

func (o atomicOutcome) acknowledgement() operation.Reply[struct{}] {
	return atomicReply(o.complete, func(struct{}) ([]byte, error) { return encodeAtomicResult(o.method, struct{}{}) })
}

func (o atomicOutcome) sessionView(response func(*protocol.Session) any) operation.Reply[sessions.View] {
	return atomicReply(o.complete, func(result sessions.View) ([]byte, error) {
		view := presentSession(result)
		return encodeAtomicResult(o.method, response(&view))
	})
}

func (o atomicOutcome) project() operation.Reply[project.Project] {
	return atomicReply(o.complete, func(result project.Project) ([]byte, error) {
		return encodeAtomicResult(o.method, presentProject(result))
	})
}

func (o atomicOutcome) commitCreation(ctx context.Context, creation sessions.Creation, err error) error {
	if err != nil {
		return o.commitFailure(ctx, err)
	}
	view := presentSession(creation.View)
	payload, err := encodeAtomicResult(o.method, &view)
	if err != nil {
		return err
	}
	err = o.complete.Commit(ctx, func(tx context.Context) ([]byte, error) {
		return payload, creation.Commit(tx)
	})
	if err == nil {
		creation.Publish()
	}
	return err
}

func (o atomicOutcome) settleRefusal(ctx context.Context, err error) error {
	if err == nil || o.complete.Attempted() {
		return err
	}
	return o.commitFailure(ctx, err)
}

func (o atomicOutcome) commitFailure(ctx context.Context, cause error) error {
	return o.complete.Commit(ctx, func(context.Context) ([]byte, error) {
		return encodeStoredOutcome(failed(projectError(cause)))
	})
}

func directReply[T any](reply *T) operation.Reply[T] {
	return func(ctx context.Context, write func(context.Context) (T, error)) error {
		value, err := write(ctx)
		if err != nil {
			return err
		}
		*reply = value
		return nil
	}
}

func atomicReply[T any](complete *operation.AtomicCompletion, encode func(T) ([]byte, error)) operation.Reply[T] {
	return func(ctx context.Context, write func(context.Context) (T, error)) error {
		return complete.Commit(ctx, func(tx context.Context) ([]byte, error) {
			value, err := write(tx)
			if err != nil {
				return nil, err
			}
			return encode(value)
		})
	}
}

func encodeAtomicResult(method *Method, value any) ([]byte, error) {
	if err := method.Meta.ValidateResult(value); err != nil {
		return nil, err
	}
	return encodeStoredOutcome(protocol.OperationResult{Value: value})
}

func replayOutcome(method *Method, payload []byte) protocol.OperationResult {
	var stored storedOutcome
	if err := decodeStoredJSON(payload, &stored); err != nil {
		return failed(projectError(fmt.Errorf("idempotency: decode stored outcome: %w", err)))
	}
	if (len(stored.Value) == 0) == (stored.Problem == nil) {
		return failed(projectError(errors.New("idempotency: stored outcome must contain exactly one value or problem")))
	}
	if stored.Problem != nil {
		return failed(failureFromData(*stored.Problem))
	}
	value, err := method.Meta.DecodeResult(stored.Value)
	if err != nil {
		return failed(projectError(fmt.Errorf("idempotency: decode stored result: %w", err)))
	}
	return protocol.OperationResult{Value: value}
}

func encodeStoredOutcome(result protocol.OperationResult) ([]byte, error) {
	var stored storedOutcome
	if result.Failure != nil {
		problem := result.Failure.Problem()
		stored.Problem = &problem
	} else {
		encoded, err := json.Marshal(result.Value, json.Deterministic(true))
		if err != nil {
			return nil, err
		}
		stored.Value = encoded
	}
	return json.Marshal(stored, json.Deterministic(true))
}

func decodeStoredJSON(encoded []byte, target any) error {
	return json.Unmarshal(encoded, target, json.RejectUnknownMembers(true))
}

func operationFingerprint(name Name, parameters any) (string, error) {
	// Two requests carrying the same parameters must hash alike, and
	// encoding/json/v2 leaves map members in iteration order.
	encoded, err := json.Marshal(parameters, json.Deterministic(true))
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(name))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(encoded)
	return hex.EncodeToString(hash.Sum(nil)), nil
}
