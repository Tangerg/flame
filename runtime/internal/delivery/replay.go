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
	framecall "github.com/Tangerg/frame/call"

	"go.opentelemetry.io/otel/trace"
)

// storedOutcome carries no layout version. It decodes strictly against the
// method's current result type, so a record this build cannot read is refused
// as a stored-outcome failure rather than misread.
type storedOutcome struct {
	Value   jsontext.Value        `json:"value,omitzero"`
	Problem *protocol.ProblemData `json:"problem,omitzero"`
}

// keyedCall is one keyed invocation of a replay-protected command.
type keyedCall struct {
	replay      *operation.Replay
	method      *Method
	key         string
	fingerprint string
}

// ownedReplay settles a keyed call in the transaction that commits the command's
// mutation instead of recording its response afterwards, so a lost
// acknowledgement can never separate the two.
type ownedReplay func(target any, ctx context.Context, parameters any, call keyedCall) protocol.OperationResult

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

// result projects the key's stored outcome, or why the key cannot serve one.
func (c keyedCall) result(ctx context.Context, payload []byte, err error) protocol.OperationResult {
	switch {
	case err == nil:
		return replayOutcome(c.method, payload)
	case errors.Is(err, framecall.ErrConflict):
		return failed(protocol.NewFailure(protocol.ErrIdempotencyConflict, "idempotency key is already bound to another operation"))
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
	slog.ErrorContext(ctx, "delivery: idempotency receipt is unconfirmed after a completed operation", "method", c.method.Meta.Name, "error", err)
	trace.SpanFromContext(ctx).RecordError(fmt.Errorf("idempotency: store %s replay: %w", c.method.Meta.Name, err))
}

func (c keyedCall) invokeAtomic(ctx context.Context, commit func(atomicOutcome) error) protocol.OperationResult {
	payload, err := c.replay.InvokeAtomic(ctx, c.key, c.fingerprint, func(complete *operation.AtomicCompletion) error {
		return commit(atomicOutcome{method: c.method, complete: complete})
	})
	return c.result(ctx, payload, err)
}

// atomicReplay settles a command whose owner commits its reply through one
// atomic completion.
func atomicReplay[Capability, Params any](commit func(Capability, context.Context, Params, atomicOutcome) error) func(Capability, context.Context, Params, keyedCall) protocol.OperationResult {
	return func(owner Capability, ctx context.Context, in Params, call keyedCall) protocol.OperationResult {
		return call.invokeAtomic(ctx, func(outcome atomicOutcome) error {
			return commit(owner, ctx, in, outcome)
		})
	}
}

// atomicOutcome is a keyed command's borrowed completion and the method whose
// result shape it stores.
type atomicOutcome struct {
	method   *Method
	complete *operation.AtomicCompletion
}

// acknowledgement stores the empty reply of a command that returns nothing.
func (o atomicOutcome) acknowledgement() operation.Reply[struct{}] {
	return atomicReply(o.complete, func(struct{}) ([]byte, error) { return encodeAtomicResult(o.method, struct{}{}) })
}

func (o atomicOutcome) sessionView(response func(*protocol.Session) any) operation.Reply[sessions.View] {
	return atomicReply(o.complete, func(result sessions.View) ([]byte, error) {
		view := presentSession(result)
		return encodeAtomicResult(o.method, response(&view))
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

// settleRefusal stores a refusal the command returned before attempting its
// write; a failed write attempt stays retryable.
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

// directReply keeps an unkeyed command's committed result as its reply.
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

// atomicReply stores a keyed command's encoded reply in its write's transaction.
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

// A replayed receipt must name the exact stored shape: the decoder refuses an
// unknown member, a repeated one, and anything after the value.
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
