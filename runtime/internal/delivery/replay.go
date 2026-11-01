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
	"github.com/Tangerg/qimu/portable"
	"go.opentelemetry.io/otel/trace"
)

// storedOutcome carries no layout version. It decodes strictly against the
// method's current result type, so a record this build cannot read is refused
// as a stored-outcome failure rather than misread.
type storedOutcome struct {
	Value   jsontext.Value        `json:"value,omitzero"`
	Problem *protocol.ProblemData `json:"problem,omitzero"`
}

func (e *Endpoint) replayOperation(ctx context.Context, method *Method, parameters any, key string, execute func() Result) Result {
	fingerprint, err := operationFingerprint(method.Meta.Name, parameters)
	if err != nil {
		return failed(projectError(fmt.Errorf("idempotency: fingerprint operation: %w", err)))
	}
	var result Result
	executed := false
	var payload []byte
	if handler, ok := e.target.(*Handler); ok && (method.Meta.Name == SessionsCreate || method.Meta.Name == sessionsFork) {
		payload, err = e.idempotency.InvokeAtomic(ctx, key, fingerprint, func() (operation.PreparedOutcome, error) {
			return handler.prepareSessionOutcome(ctx, method, parameters)
		})
	} else {
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
		case errors.Is(err, portable.ErrInvocationConflict):
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

func (h *Handler) prepareSessionOutcome(ctx context.Context, method *Method, request any) (operation.PreparedOutcome, error) {
	var creation sessions.Creation
	var err error
	switch in := request.(type) {
	case protocol.CreateSessionRequest:
		creation, err = h.prepareCreate(ctx, in)
	case protocol.ForkSessionRequest:
		creation, err = h.prepareFork(ctx, in)

	default:
		return operation.PreparedOutcome{}, fmt.Errorf("idempotency: invalid Session creation request %T", request)
	}
	if err != nil {
		payload, encodeErr := encodeStoredOutcome(failed(projectError(err)))
		return operation.PreparedOutcome{Payload: payload}, encodeErr
	}
	view := presentSession(creation.View)
	if err := method.Meta.ValidateResult(&view); err != nil {
		return operation.PreparedOutcome{}, err
	}
	payload, err := encodeStoredOutcome(Result{Value: &view})
	return operation.PreparedOutcome{Payload: payload, Commit: creation.Commit, Publish: creation.Publish}, err
}

func replayOutcome(method *Method, payload []byte) Result {
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
	return Result{Value: value}
}

func encodeStoredOutcome(result Result) ([]byte, error) {
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
