package delivery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"sort"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"encoding/json/jsontext"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/internal/keylock"
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	maxIdempotencyKeyBytes       = 255
	idempotencyStoreWriteTimeout = 5 * time.Second
)

type replayStore struct {
	store idempotency.Store
	// inFlight serializes one key's execution and receipt for as long as the
	// caller's own request lasts.
	inFlight  *keylock.Set
	pendingMu sync.Mutex
	pending   map[string]idempotency.Record
}

// storedOutcome carries no layout version. It decodes strictly against the
// method's current result type, so a record this build cannot read is refused
// as a stored-outcome failure rather than misread.
type storedOutcome struct {
	Value   jsontext.Value        `json:"value,omitzero"`
	Problem *protocol.ProblemData `json:"problem,omitzero"`
}

func newReplayStore(store idempotency.Store) *replayStore {
	return &replayStore{
		store:    store,
		inFlight: keylock.NewSet(),
		pending:  make(map[string]idempotency.Record),
	}
}

func (r *replayStore) invoke(
	ctx context.Context,
	method *Method,
	parameters any,
	key string,
	execute func() Result,
	target any,
) Result {
	if len(key) > maxIdempotencyKeyBytes {
		return failed(NewFailure(protocol.ErrInvalidParams, "idempotency key must not exceed 255 bytes"))
	}
	fingerprint, err := operationFingerprint(method.Meta.Name, parameters)
	if err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: fingerprint operation: %w", err)))
	}
	release, err := r.inFlight.Acquire(ctx, key)
	if err != nil {
		return failed(ProjectError(err))
	}
	defer release()

	if pending, ok := r.pendingCompletion(key); ok {
		if pending.Fingerprint != fingerprint {
			return failed(NewFailure(protocol.ErrIdempotencyConflict, "idempotency key is already bound to another operation"))
		}
		completionErr := r.completeDetached(ctx, pending)
		if completionErr != nil {
			// The caller learns only that the receipt is unsettled; the reason it
			// could not be settled belongs to whoever has to fix the store.
			slog.ErrorContext(ctx, "delivery: settle idempotency receipt",
				"method", method.Meta.Name, "error", completionErr)
			return failed(r.persistenceFailure(completionErr))
		}
		r.forgetPendingCompletion(key, fingerprint)
		return r.replay(ctx, method, pending.Payload, target)
	}

	record, claimed, err := r.store.Claim(ctx, key, fingerprint)
	if err != nil {
		if errors.Is(err, idempotency.ErrKeyConflict) {
			return failed(NewFailure(protocol.ErrIdempotencyConflict, "idempotency key is already bound to another operation"))
		}
		return failed(ProjectError(fmt.Errorf("idempotency: claim replay key: %w", err)))
	}
	if !claimed {
		if len(record.Payload) == 0 {
			return failed(NewFailure(protocol.ErrIdempotencyInProgress, "command outcome is not yet known; retain the original request identity"))
		}
		return r.replay(ctx, method, record.Payload, target)
	}

	result := execute()
	payload, err := encodeStoredOutcome(result)
	if err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: encode operation outcome: %w", err)))
	}
	record = idempotency.Record{Key: key, Fingerprint: fingerprint, Payload: payload}
	if err := r.completeDetached(ctx, record); err != nil {
		if errors.Is(err, idempotency.ErrKeyConflict) {
			return failed(NewFailure(protocol.ErrIdempotencyConflict, "idempotency key is already bound to another operation"))
		}
		r.rememberPendingCompletion(record)
		// The execution outcome is known even though its receipt is unconfirmed.
		// Retain it for completion without repeating the command or choosing a
		// different outcome, including while shutdown settlement is refused.
		slog.ErrorContext(ctx, "delivery: idempotency receipt is unconfirmed after a completed operation",
			"method", method.Meta.Name, "error", err)
		trace.SpanFromContext(ctx).RecordError(fmt.Errorf("idempotency: store replay: %w", err))
		return failed(r.persistenceFailure(err))
	}
	return result
}

func (r *replayStore) replay(ctx context.Context, method *Method, payload []byte, target any) Result {
	var stored storedOutcome
	if err := decodeStoredJSON(payload, &stored); err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: decode stored outcome: %w", err)))
	}
	if (len(stored.Value) == 0) == (stored.Problem == nil) {
		return failed(ProjectError(errors.New("idempotency: stored outcome must contain exactly one value or problem")))
	}
	if stored.Problem != nil {
		return failed(failureFromData(*stored.Problem))
	}
	value, err := method.Meta.DecodeResult(stored.Value)
	if err != nil {
		return failed(ProjectError(fmt.Errorf("idempotency: decode stored result: %w", err)))
	}
	result := Result{Value: value}
	if method.Meta.Idempotency != IdempotencyReplayRunStream {
		return result
	}

	runID, segmentID, ok := runOpeningIdentity(value)
	if !ok {
		return failed(ProjectError(errors.New("idempotency: stored run-opening result has an invalid shape")))
	}
	subscriber, ok := target.(interface {
		SubscribeRun(context.Context, protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	})
	if !ok || !capabilityAvailable(subscriber) {
		return failed(ProjectError(errors.New("operation: target cannot handle runs.subscribe")))
	}
	_, events, err := subscriber.SubscribeRun(ctx, protocol.SubscribeRunRequest{RunID: runID, SegmentID: segmentID})
	switch {
	case unattachable(err):
		result.Events = emptyEventStream
		return result
	case err != nil:
		return failed(ProjectError(err))
	default:
		result.Events = validateEvents(ctx, method.Meta.Event, eraseEventType(events))
		return result
	}
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

func runOpeningIdentity(value any) (runID, segmentID string, ok bool) {
	switch opening := value.(type) {
	case *protocol.StartRunResponse:
		if opening != nil {
			return opening.RunID, opening.SegmentID, true
		}
	case *protocol.ResumeRunResponse:
		if opening != nil {
			return opening.RunID, opening.SegmentID, true
		}
	}
	return "", "", false
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

func (r *replayStore) completeDetached(ctx context.Context, record idempotency.Record) error {
	return r.completeWithin(context.WithoutCancel(ctx), record)
}

func (r *replayStore) completeWithin(ctx context.Context, record idempotency.Record) error {
	writeContext, cancel := context.WithTimeout(ctx, idempotencyStoreWriteTimeout)
	defer cancel()
	return r.store.Complete(writeContext, record)
}

// flushPending persists every business outcome already known to this Endpoint.
// It runs only after admission is closed and all accepted invocations have
// returned, so no new pending record can appear while the process owner is
// deciding whether dependencies are safe to close. Failed records stay in the
// map so a later Close can retry without executing their commands again.
func (r *replayStore) flushPending(ctx context.Context) error {
	if ctx == nil {
		return errors.New("idempotency: flush context is required")
	}
	var errs []error
	for _, key := range r.pendingKeys() {
		release, err := r.inFlight.Acquire(ctx, key)
		if err != nil {
			errs = append(errs, fmt.Errorf("idempotency: flush pending outcome: %w", err))
			continue
		}
		pending, ok := r.pendingCompletion(key)
		if ok {
			err = r.completeWithin(ctx, pending)
			if err == nil {
				r.forgetPendingCompletion(key, pending.Fingerprint)
			} else {
				errs = append(errs, fmt.Errorf("idempotency: flush pending outcome: %w", err))
			}
		}
		release()
	}
	return errors.Join(errs...)
}

func (r *replayStore) persistenceFailure(err error) *Failure {
	if errors.Is(err, idempotency.ErrKeyConflict) {
		return NewFailure(protocol.ErrIdempotencyConflict, "idempotency key is already bound to another operation")
	}
	if errors.Is(err, idempotency.ErrClaimLost) || errors.Is(err, idempotency.ErrOutcomeConflict) {
		return ProjectError(fmt.Errorf("idempotency: persist execution outcome: %w", err))
	}
	return NewFailure(protocol.ErrIdempotencyInProgress, "operation outcome persistence is still pending")
}

func (r *replayStore) pendingCompletion(key string) (idempotency.Record, bool) {
	r.pendingMu.Lock()
	defer r.pendingMu.Unlock()
	record, ok := r.pending[key]
	record.Payload = bytes.Clone(record.Payload)
	return record, ok
}

func (r *replayStore) rememberPendingCompletion(record idempotency.Record) {
	record.Payload = bytes.Clone(record.Payload)
	r.pendingMu.Lock()
	r.pending[record.Key] = record
	r.pendingMu.Unlock()
}

func (r *replayStore) forgetPendingCompletion(key, fingerprint string) {
	r.pendingMu.Lock()
	if r.pending[key].Fingerprint == fingerprint {
		delete(r.pending, key)
	}
	r.pendingMu.Unlock()
}

func (r *replayStore) pendingKeys() []string {
	r.pendingMu.Lock()
	keys := make([]string, 0, len(r.pending))
	for key := range r.pending {
		keys = append(keys, key)
	}
	r.pendingMu.Unlock()
	sort.Strings(keys)
	return keys
}

func unattachable(err error) bool {
	return errors.Is(err, protocol.ErrRunNotFound) ||
		errors.Is(err, protocol.ErrRunWaiting) ||
		errors.Is(err, protocol.ErrRunFinished) ||
		errors.Is(err, protocol.ErrStaleSegment)
}

var emptyEventStream iter.Seq2[any, error] = func(func(any, error) bool) {}

type memoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]memoryIdempotencyRecord
}

type memoryIdempotencyRecord struct {
	idempotency.Record
	expiresAt time.Time
}

func newMemoryIdempotencyStore() *memoryIdempotencyStore {
	return &memoryIdempotencyStore{records: make(map[string]memoryIdempotencyRecord)}
}

func (m *memoryIdempotencyStore) Claim(_ context.Context, key, fingerprint string) (idempotency.Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for storedKey, stored := range m.records {
		if len(stored.Payload) != 0 && !now.Before(stored.expiresAt) {
			delete(m.records, storedKey)
		}
	}
	stored, ok := m.records[key]
	if ok {
		if stored.Fingerprint != fingerprint {
			return idempotency.Record{}, false, idempotency.ErrKeyConflict
		}
		stored.Payload = bytes.Clone(stored.Payload)
		return stored.Record, false, nil
	}
	record := idempotency.Record{Key: key, Fingerprint: fingerprint}
	m.records[key] = memoryIdempotencyRecord{Record: record}
	return record, true, nil
}

func (m *memoryIdempotencyStore) Complete(_ context.Context, record idempotency.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored, ok := m.records[record.Key]
	if !ok {
		return idempotency.ErrClaimLost
	}
	if stored.Fingerprint != record.Fingerprint {
		return idempotency.ErrKeyConflict
	}
	if len(stored.Payload) != 0 {
		if !time.Now().Before(stored.expiresAt) {
			delete(m.records, record.Key)
			return idempotency.ErrClaimLost
		}
		if !bytes.Equal(stored.Payload, record.Payload) {
			return idempotency.ErrOutcomeConflict
		}
		return nil
	}
	stored.Payload = bytes.Clone(record.Payload)
	stored.expiresAt = time.Now().Add(idempotency.Retention)
	m.records[record.Key] = stored
	return nil
}
