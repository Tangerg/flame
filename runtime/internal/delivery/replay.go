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

func (e *Endpoint) replayOperation(ctx context.Context, method *Method, parameters any, key string, execute func() protocol.OperationResult) protocol.OperationResult {
	fingerprint, err := operationFingerprint(method.Meta.Name, parameters)
	if err != nil {
		return failed(projectError(fmt.Errorf("idempotency: fingerprint operation: %w", err)))
	}
	var result protocol.OperationResult
	executed := false
	var payload []byte
	handler, ownsOutcome := e.target.(*Handler)
	switch {
	case ownsOutcome && method.Meta.Name == sessionsRollback:
		payload, err = e.replayRollback(ctx, handler, method, parameters.(protocol.RollbackSessionRequest), key, fingerprint)
	case ownsOutcome && hasAtomicOutcome(method.Meta.Name):
		payload, err = e.idempotency.InvokeAtomic(ctx, key, fingerprint, func(complete *operation.AtomicCompletion) error {
			return handler.commitAtomicOutcome(ctx, method, parameters, complete)
		})
	default:
		payload, err = e.idempotency.Invoke(ctx, key, fingerprint, func() ([]byte, error) {
			result = execute()
			executed = true
			return encodeStoredOutcome(result)
		})
	}
	if err != nil {
		if executed || errors.Is(err, operation.ErrOutcomePending) {
			slog.ErrorContext(ctx, "delivery: idempotency receipt is unconfirmed after a completed operation", "method", method.Meta.Name, "error", err)
			trace.SpanFromContext(ctx).RecordError(fmt.Errorf("idempotency: store %s replay: %w", method.Meta.Name, err))
		}
		switch {
		case errors.Is(err, framecall.ErrConflict):
			return failed(protocol.NewFailure(protocol.ErrIdempotencyConflict, "idempotency key is already bound to another operation"))
		case errors.Is(err, operation.ErrOutcomePending):
			return failed(protocol.NewFailure(protocol.ErrIdempotencyInProgress, operation.ErrOutcomePending.Error()))
		case errors.Is(err, operation.ErrInProgress):
			return failed(protocol.NewFailure(protocol.ErrIdempotencyInProgress, operation.ErrInProgress.Error()))
		default:
			return failed(projectError(err))
		}
	}
	if executed {
		return result
	}
	return replayOutcome(method, payload)
}

func hasAtomicOutcome(name Name) bool {
	switch name {
	case SessionsCreate, sessionsFork, feedbackCreate:
		return true
	default:
		return false
	}
}

func (s *Handler) commitAtomicOutcome(ctx context.Context, method *Method, request any, complete *operation.AtomicCompletion) error {
	var creation sessions.Creation
	var err error
	switch in := request.(type) {
	case protocol.FeedbackRequest:
		write, err := s.feedback.Prepare(ctx, feedbackCommand(in))
		if err != nil {
			return err
		}
		payload, err := encodeStoredOutcome(protocol.OperationResult{Value: struct{}{}})
		if err != nil {
			return err
		}
		return complete.Commit(ctx, func(tx context.Context) ([]byte, error) {
			return payload, write(tx)
		})
	case protocol.CreateSessionRequest:
		creation, err = s.prepareCreate(ctx, in)
	case protocol.ForkSessionRequest:
		creation, err = s.prepareFork(ctx, in)
	default:
		return fmt.Errorf("idempotency: unsupported atomic command input %T", request)
	}
	if err != nil {
		return commitAtomicFailure(ctx, complete, err)
	}
	view := presentSession(creation.View)
	payload, err := encodeAtomicResult(method, &view)
	if err != nil {
		return err
	}
	err = complete.Commit(ctx, func(tx context.Context) ([]byte, error) {
		return payload, creation.Commit(tx)
	})
	if err == nil {
		creation.Publish()
	}
	return err
}

func (s *Handler) commitRollbackOutcome(ctx context.Context, method *Method, in protocol.RollbackSessionRequest, complete *operation.AtomicCompletion) error {
	err := s.rollbackHistory(ctx, in, func(ctx context.Context, write func(context.Context) (sessions.RollbackResult, error)) error {
		return complete.Commit(ctx, func(tx context.Context) ([]byte, error) {
			result, err := write(tx)
			if err != nil {
				return nil, err
			}
			return encodeAtomicResult(method, presentRollbackResult(result))
		})
	})
	if err == nil || complete.Attempted() {
		return err
	}
	return commitAtomicFailure(ctx, complete, err)
}

func commitAtomicFailure(ctx context.Context, complete *operation.AtomicCompletion, cause error) error {
	return complete.Commit(ctx, func(context.Context) ([]byte, error) {
		return encodeStoredOutcome(failed(projectError(cause)))
	})
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
