package delivery

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

type countingCancelService struct {
	calls atomic.Int32
}

func (c *countingCancelService) CancelRun(_ context.Context, request protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	c.calls.Add(1)
	outcome := protocol.RunOutcome{Type: protocol.OutcomeCanceled}
	finishedAt := time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC)
	return &protocol.CancelRunResponse{
		Type: protocol.CancelRunRoot,
		Run: protocol.RunRef{RunSummary: protocol.RunSummary{
			ID: request.RunID, SessionID: "ses_1", Provider: "mock", Model: "balanced",
			Status: protocol.RunStatusFinished, Outcome: &outcome,
			CreatedAt: finishedAt.Add(-time.Second), FinishedAt: finishedAt,
		}},
	}, nil
}

type flakyCompletionStore struct {
	idempotency.Store
	failures atomic.Int32
}

type claimLostOnceStore struct {
	backing *memoryIdempotencyStore
	once    sync.Once
}

type competingCompletionStore struct {
	backing        *memoryIdempotencyStore
	durablePayload []byte
	durableErr     error
	once           sync.Once
}

type cancellationAwareCompletionStore struct {
	idempotency.Store
	attempts atomic.Int32
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (c *claimLostOnceStore) Claim(
	ctx context.Context,
	key string,
	fingerprint string,
) (idempotency.Record, bool, error) {
	return c.backing.Claim(ctx, key, fingerprint)
}

func (c *claimLostOnceStore) Complete(ctx context.Context, record idempotency.Record) error {
	lost := false
	c.once.Do(func() {
		c.backing.mu.Lock()
		delete(c.backing.records, record.Key)
		c.backing.mu.Unlock()
		lost = true
	})
	if lost {
		return idempotency.ErrClaimLost
	}
	return c.backing.Complete(ctx, record)
}

func (c *competingCompletionStore) Claim(
	ctx context.Context,
	key string,
	fingerprint string,
) (idempotency.Record, bool, error) {
	return c.backing.Claim(ctx, key, fingerprint)
}

func (c *competingCompletionStore) Complete(ctx context.Context, record idempotency.Record) error {
	intercepted := false
	c.once.Do(func() {
		intercepted = true
		durable := record
		durable.Payload = c.durablePayload
		c.durableErr = c.backing.Complete(ctx, durable)
	})
	if intercepted {
		if c.durableErr != nil {
			return c.durableErr
		}
		return errors.New("completion acknowledgement was lost")
	}
	return c.backing.Complete(ctx, record)
}

func (c *cancellationAwareCompletionStore) Complete(ctx context.Context, record idempotency.Record) error {
	if c.attempts.Add(1) == 1 {
		return errors.New("temporary completion failure")
	}
	c.once.Do(func() { close(c.entered) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.release:
		return c.Store.Complete(ctx, record)
	}
}

func (f *flakyCompletionStore) Complete(ctx context.Context, record idempotency.Record) error {
	if f.failures.Add(-1) >= 0 {
		return errors.New("temporary completion failure")
	}
	return f.Store.Complete(ctx, record)
}

func TestEndpointRejectsIdempotencyStoreMismatchBeforeBusinessAdmission(t *testing.T) {
	service := &countingCancelService{}
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyNamespace: testsupport.IdempotencyNamespace})
	request := protocol.CancelRunRequest{RunID: "run_1"}

	refused := endpoint.Invoke(t.Context(), "runs.cancel", request, Options{
		IdempotencyKey:       "cancel-once",
		IdempotencyNamespace: testsupport.AlternateIdempotencyNamespace,
	})
	if !errors.Is(refused.Failure, protocol.ErrIdempotencyStoreMismatch) {
		t.Fatalf("mismatch error = %v, want idempotency_store_mismatch", refused.Failure)
	}
	if got := service.calls.Load(); got != 0 {
		t.Fatalf("business calls after mismatch = %d, want 0", got)
	}

	accepted := endpoint.Invoke(t.Context(), "runs.cancel", request, Options{
		IdempotencyKey:       "cancel-once",
		IdempotencyNamespace: testsupport.IdempotencyNamespace,
	})
	if accepted.Failure != nil {
		t.Fatalf("matching namespace error = %v", accepted.Failure)
	}
	if got := service.calls.Load(); got != 1 {
		t.Fatalf("business calls after match = %d, want 1", got)
	}
}

func TestOperationFingerprintUsesTypedSemanticValue(t *testing.T) {
	t.Parallel()

	parameters := protocol.CreateSessionRequest{
		Title:     "session",
		Workspace: &protocol.WorkspaceRef{Path: "/workspace"},
	}
	first, err := operationFingerprint("sessions.create", parameters)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	second, err := operationFingerprint("sessions.create", parameters)
	if err != nil {
		t.Fatalf("fingerprint again: %v", err)
	}
	if first != second {
		t.Fatalf("same semantic parameters produced %q and %q", first, second)
	}
}

func TestMemoryIdempotencyStoreKeepsAbandonedClaimReserved(t *testing.T) {
	store := newMemoryIdempotencyStore()
	record, claimed, err := store.Claim(t.Context(), "abandoned", "first")
	if err != nil || !claimed {
		t.Fatalf("initial claim = (%+v, %v, %v)", record, claimed, err)
	}
	store.mu.Lock()
	aged := store.records[record.Key]
	aged.expiresAt = time.Time{}
	store.records[record.Key] = aged
	store.mu.Unlock()

	got, claimed, err := store.Claim(t.Context(), record.Key, record.Fingerprint)
	if err != nil || claimed || len(got.Payload) != 0 {
		t.Fatalf("aged pending claim = (%+v, %v, %v), want reserved", got, claimed, err)
	}
	if _, _, claimErr := store.Claim(t.Context(), record.Key, "second"); !errors.Is(claimErr, idempotency.ErrKeyConflict) {
		t.Fatalf("reuse aged pending claim = %v, want ErrKeyConflict", claimErr)
	}
	record.Payload = []byte(`{"version":1}`)
	if completeErr := store.Complete(t.Context(), record); completeErr != nil {
		t.Fatalf("complete aged pending claim: %v", completeErr)
	}
	got, claimed, err = store.Claim(t.Context(), record.Key, record.Fingerprint)
	if err != nil || claimed || string(got.Payload) != string(record.Payload) {
		t.Fatalf("completed aged claim = (%+v, %v, %v)", got, claimed, err)
	}
	store.mu.Lock()
	aged = store.records[record.Key]
	aged.expiresAt = time.Time{}
	store.records[record.Key] = aged
	store.mu.Unlock()
	got, claimed, err = store.Claim(t.Context(), record.Key, "second")
	if err != nil || !claimed || got.Fingerprint != "second" {
		t.Fatalf("replace expired result = (%+v, %v, %v)", got, claimed, err)
	}
}

func TestMemoryIdempotencyStorePrunesExpiredResultsBeforeNewClaim(t *testing.T) {
	store := newMemoryIdempotencyStore()
	expired, claimed, err := store.Claim(t.Context(), "expired", "first")
	if err != nil || !claimed {
		t.Fatalf("claim expired fixture = (%+v, %v, %v)", expired, claimed, err)
	}
	expired.Payload = []byte(`{"version":1}`)
	if err := store.Complete(t.Context(), expired); err != nil {
		t.Fatalf("complete expired fixture: %v", err)
	}
	store.mu.Lock()
	stored := store.records[expired.Key]
	stored.expiresAt = time.Time{}
	store.records[expired.Key] = stored
	store.mu.Unlock()

	if _, claimed, err := store.Claim(t.Context(), "fresh", "second"); err != nil || !claimed {
		t.Fatalf("claim fresh key = (%v, %v)", claimed, err)
	}
	store.mu.Lock()
	_, exists := store.records[expired.Key]
	count := len(store.records)
	store.mu.Unlock()
	if exists || count != 1 {
		t.Fatalf("records after fresh claim = %d, expired exists = %v", count, exists)
	}
}

func TestCompletionFailureRetriesWithoutRepeatingCommand(t *testing.T) {
	service := &countingCancelService{}
	store := &flakyCompletionStore{Store: newMemoryIdempotencyStore()}
	store.failures.Store(1)
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: store})
	options := Options{IdempotencyKey: "cancel-once"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	for attempt := range 2 {
		response, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options)
		if err != nil || response.Run.ID != "run_1" {
			t.Fatalf("replay %d = (%+v, %v)", attempt, response, err)
		}
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("CancelRun calls = %d, want 1", calls)
	}
}

func TestAwaitShutdownFlushesKnownCompletionBeforeStoreClosure(t *testing.T) {
	service := &countingCancelService{}
	backing := newMemoryIdempotencyStore()
	store := &flakyCompletionStore{Store: backing}
	store.failures.Store(1)
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: store})
	options := Options{IdempotencyKey: "flush-on-shutdown"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	endpoint.BeginShutdown()
	if awaitShutdownErr := endpoint.AwaitShutdown(t.Context()); awaitShutdownErr != nil {
		t.Fatalf("AwaitShutdown: %v", awaitShutdownErr)
	}

	reopenedService := &countingCancelService{}
	reopened := mustNewEndpoint(t, reopenedService, EndpointConfig{IdempotencyStore: backing})
	response, err := reopened.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options)
	if err != nil || response.Run.ID != "run_1" {
		t.Fatalf("replay after graceful shutdown = (%+v, %v)", response, err)
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("original CancelRun calls = %d, want 1", calls)
	}
	if calls := reopenedService.calls.Load(); calls != 0 {
		t.Fatalf("reopened CancelRun calls = %d, want 0", calls)
	}
}

func TestAwaitShutdownKeepsFailedPendingCompletionForRetry(t *testing.T) {
	service := &countingCancelService{}
	backing := newMemoryIdempotencyStore()
	store := &flakyCompletionStore{Store: backing}
	store.failures.Store(2)
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: store})
	options := Options{IdempotencyKey: "retry-shutdown-flush"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	endpoint.BeginShutdown()
	if err := endpoint.AwaitShutdown(t.Context()); err == nil {
		t.Fatal("first AwaitShutdown succeeded while completion persistence failed")
	}
	if err := endpoint.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("retry AwaitShutdown: %v", err)
	}

	reopenedService := &countingCancelService{}
	reopened := mustNewEndpoint(t, reopenedService, EndpointConfig{IdempotencyStore: backing})
	if _, err := reopened.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options); err != nil {
		t.Fatalf("replay after retried shutdown flush: %v", err)
	}
	if calls := reopenedService.calls.Load(); calls != 0 {
		t.Fatalf("reopened CancelRun calls = %d, want 0", calls)
	}
}

func TestAwaitShutdownFlushHonorsOwnerCancellation(t *testing.T) {
	backing := newMemoryIdempotencyStore()
	store := &cancellationAwareCompletionStore{
		Store: backing, entered: make(chan struct{}), release: make(chan struct{}),
	}
	endpoint := mustNewEndpoint(t, &countingCancelService{}, EndpointConfig{IdempotencyStore: store})
	options := Options{IdempotencyKey: "cancel-shutdown-flush"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	endpoint.BeginShutdown()
	waitCtx, cancelWait := context.WithCancel(t.Context())
	waitErr := make(chan error, 1)
	go func() { waitErr <- endpoint.AwaitShutdown(waitCtx) }()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("shutdown flush did not enter completion store")
	}
	cancelWait()
	select {
	case err := <-waitErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("AwaitShutdown cancellation = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("AwaitShutdown did not propagate owner cancellation")
	}
	close(store.release)
	if err := endpoint.AwaitShutdown(t.Context()); err != nil {
		t.Fatalf("retry AwaitShutdown: %v", err)
	}
}

func TestLostCompletionClaimIsReacquiredWithoutRepeatingCommand(t *testing.T) {
	service := &countingCancelService{}
	store := &claimLostOnceStore{backing: newMemoryIdempotencyStore()}
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: store})
	options := Options{IdempotencyKey: "recover-lost-claim"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	const callers = 16
	errCh := make(chan error, callers)
	var callersDone sync.WaitGroup
	for range callers {
		callersDone.Add(1)
		go func() {
			defer callersDone.Done()
			response, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options)
			if err != nil {
				errCh <- err
				return
			}
			if response.Run.ID != "run_1" {
				errCh <- fmt.Errorf("replayed run = %q, want run_1", response.Run.ID)
			}
		}()
	}
	callersDone.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("recovered replay: %v", err)
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("CancelRun calls = %d, want 1", calls)
	}
}

func TestPendingCompletionReplaysDurableFirstResult(t *testing.T) {
	durableFinishedAt := time.Date(2026, 8, 11, 2, 0, 0, 0, time.UTC)
	durableOutcome := protocol.RunOutcome{Type: protocol.OutcomeCanceled}
	durablePayload, err := encodeStoredOutcome(Result{Value: &protocol.CancelRunResponse{
		Type: protocol.CancelRunRoot,
		Run: protocol.RunRef{RunSummary: protocol.RunSummary{
			ID: "run_1", SessionID: "ses_1", Provider: "mock", Model: "balanced",
			Status: protocol.RunStatusFinished, Outcome: &durableOutcome,
			CreatedAt: durableFinishedAt.Add(-time.Second), FinishedAt: durableFinishedAt,
		}},
	}})
	if err != nil {
		t.Fatalf("encode durable outcome: %v", err)
	}
	service := &countingCancelService{}
	store := &competingCompletionStore{
		backing:        newMemoryIdempotencyStore(),
		durablePayload: durablePayload,
	}
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: store})
	options := Options{IdempotencyKey: "durable-first-result"}
	request := protocol.CancelRunRequest{RunID: "run_1"}

	_, err = endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("first call error = %v, want idempotency_in_progress", err)
	}
	response, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", request, options)
	if err != nil {
		t.Fatalf("replay durable first result: %v", err)
	}
	if !response.Run.FinishedAt.Equal(durableFinishedAt) {
		t.Fatalf("replayed FinishedAt = %v, want durable %v", response.Run.FinishedAt, durableFinishedAt)
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("CancelRun calls = %d, want 1", calls)
	}
}

func TestPendingCompletionRejectsKeyReuse(t *testing.T) {
	store := &flakyCompletionStore{Store: newMemoryIdempotencyStore()}
	store.failures.Store(1)
	endpoint := mustNewEndpoint(t, &countingCancelService{}, EndpointConfig{IdempotencyStore: store})
	options := Options{IdempotencyKey: "bound-key"}

	_, _ = endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", protocol.CancelRunRequest{RunID: "run_1"}, options)
	_, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](t.Context(), "runs.cancel", protocol.CancelRunRequest{RunID: "run_2"}, options)
	if !errors.Is(err, protocol.ErrIdempotencyConflict) {
		t.Fatalf("key reuse error = %v, want idempotency_conflict", err)
	}
}

func TestReplayRejectsUnversionedStoredOutcome(t *testing.T) {
	t.Parallel()

	method, ok := contract.lookup("runs.cancel")
	if !ok {
		t.Fatal("runs.cancel is not registered")
	}
	result := newReplayStore(newMemoryIdempotencyStore()).replay(
		t.Context(), method, []byte(`{"value":{}}`), &countingCancelService{},
	)
	if !errors.Is(result.Failure, protocol.ErrInternalError) {
		t.Fatalf("replay failure = %v, want internal_error", result.Failure)
	}
}

func TestReplayRejectsUnknownStoredOutcomeFields(t *testing.T) {
	t.Parallel()

	method, ok := contract.lookup("runs.cancel")
	if !ok {
		t.Fatal("runs.cancel is not registered")
	}
	response, err := (&countingCancelService{}).CancelRun(
		t.Context(), protocol.CancelRunRequest{RunID: "run_1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := encodeStoredOutcome(Result{Value: response})
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]jsontext.Value
	if err := json.Unmarshal(valid, &envelope); err != nil {
		t.Fatal(err)
	}

	withUnknownEnvelope := maps.Clone(envelope)
	withUnknownEnvelope["future"] = jsontext.Value(`true`)
	unknownEnvelope, err := json.Marshal(withUnknownEnvelope)
	if err != nil {
		t.Fatal(err)
	}

	var storedResult map[string]jsontext.Value
	if err := json.Unmarshal(envelope["value"], &storedResult); err != nil {
		t.Fatal(err)
	}
	storedResult["future"] = jsontext.Value(`true`)
	unknownResult, err := json.Marshal(storedResult)
	if err != nil {
		t.Fatal(err)
	}
	withUnknownResult := maps.Clone(envelope)
	withUnknownResult["value"] = unknownResult
	unknownResultPayload, err := json.Marshal(withUnknownResult)
	if err != nil {
		t.Fatal(err)
	}

	for _, payload := range [][]byte{unknownEnvelope, unknownResultPayload} {
		result := newReplayStore(newMemoryIdempotencyStore()).replay(
			t.Context(), method, payload, &countingCancelService{},
		)
		if !errors.Is(result.Failure, protocol.ErrInternalError) {
			t.Fatalf("replay failure = %v, want internal_error", result.Failure)
		}
	}
}

type countingSteerService struct{ calls atomic.Int64 }

func (s *countingSteerService) SteerRun(context.Context, protocol.SteerRunRequest) (*protocol.SteerRunResponse, error) {
	s.calls.Add(1)
	return &protocol.SteerRunResponse{UserItemID: "item_steer"}, nil
}

func TestSteerAdmissionIdentityReplaysWithoutReExecuting(t *testing.T) {
	service := &countingSteerService{}
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: newMemoryIdempotencyStore()})
	options := Options{IdempotencyKey: "steer-once"}
	request := protocol.SteerRunRequest{
		RunID: "run_1", ExpectedSegmentID: "seg_1",
		Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "wait"}},
	}

	if result, err := endpoint.Call[protocol.SteerRunRequest, *protocol.SteerRunResponse](t.Context(), "runs.steer", request, options); err != nil || result.UserItemID != "item_steer" {
		t.Fatalf("first steer: %v", err)
	}
	if result, err := endpoint.Call[protocol.SteerRunRequest, *protocol.SteerRunResponse](t.Context(), "runs.steer", request, options); err != nil || result.UserItemID != "item_steer" {
		t.Fatalf("replayed steer: %v", err)
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("SteerRun calls = %d, want 1", calls)
	}
}

// relocatingSessionService advertises the feature sessions.update needs only
// once enabled is set, so a refusal and a later admission share one key.
type relocatingSessionService struct {
	enabled atomic.Bool
	calls   atomic.Int64
}

func (r *relocatingSessionService) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{Capabilities: protocol.ServerCapabilities{
		Features: map[string]protocol.FeatureCapability{
			protocol.FeatureRelocate: {Enabled: r.enabled.Load()},
		},
	}}, nil
}

func (r *relocatingSessionService) UpdateSession(
	_ context.Context,
	request protocol.UpdateSessionRequest,
) (*protocol.Session, error) {
	r.calls.Add(1)
	at := time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC)
	return &protocol.Session{
		ID: request.SessionID, Title: "relocated", Status: protocol.SessionStatusIdle,
		Provider: "mock", Model: "balanced",
		Workspace: protocol.WorkspaceInfo{
			Ref: protocol.WorkspaceRef{Path: "/next"}, ProjectRoot: "/next",
			Availability: protocol.WorkspaceAvailable,
		},
		CreatedAt: at, UpdatedAt: at, Revision: 2,
	}, nil
}

// TestCapabilityRefusalDoesNotClaimTheIdempotencyKey: capability admission is
// this request's contract, so a refused request must leave the key free for the
// retry that can satisfy it instead of caching the refusal as the operation's
// outcome.
func TestCapabilityRefusalDoesNotClaimTheIdempotencyKey(t *testing.T) {
	service := &relocatingSessionService{}
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: newMemoryIdempotencyStore()})
	options := Options{IdempotencyKey: "relocate-once"}
	request := protocol.UpdateSessionRequest{
		SessionID: "ses_1", ExpectedRevision: 1, Workspace: &protocol.WorkspaceRef{Path: "/next"},
	}

	_, err := endpoint.Call[protocol.UpdateSessionRequest, *protocol.Session](
		t.Context(), SessionsUpdate, request, options,
	)
	var gap *CapabilityGapError
	if !errors.As(err, &gap) {
		t.Fatalf("refused update = %v, want a capability gap", err)
	}

	service.enabled.Store(true)
	updated, err := endpoint.Call[protocol.UpdateSessionRequest, *protocol.Session](
		t.Context(), SessionsUpdate, request, options,
	)
	if err != nil {
		t.Fatalf("retry under an admitted capability: %v", err)
	}
	if updated == nil || updated.ID != "ses_1" {
		t.Fatalf("retry result = %+v, want the updated Session", updated)
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("UpdateSession calls = %d, want 1", calls)
	}
}

// TestReplayServesOnlyARequestThisCapabilitySetAdmits: a stored outcome answers
// the operation, not the caller's admission, so a replay is still refused when
// this request cannot ask for it.
func TestReplayServesOnlyARequestThisCapabilitySetAdmits(t *testing.T) {
	service := &relocatingSessionService{}
	service.enabled.Store(true)
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: newMemoryIdempotencyStore()})
	options := Options{IdempotencyKey: "relocate-once"}
	request := protocol.UpdateSessionRequest{
		SessionID: "ses_1", ExpectedRevision: 1, Workspace: &protocol.WorkspaceRef{Path: "/next"},
	}

	if _, err := endpoint.Call[protocol.UpdateSessionRequest, *protocol.Session](
		t.Context(), SessionsUpdate, request, options,
	); err != nil {
		t.Fatalf("first update: %v", err)
	}

	service.enabled.Store(false)
	_, err := endpoint.Call[protocol.UpdateSessionRequest, *protocol.Session](
		t.Context(), SessionsUpdate, request, options,
	)
	var gap *CapabilityGapError
	if !errors.As(err, &gap) {
		t.Fatalf("replay without the capability = %v, want a capability gap", err)
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("UpdateSession calls = %d, want 1", calls)
	}
}

type blockingSteerService struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int64
}

func (b *blockingSteerService) SteerRun(context.Context, protocol.SteerRunRequest) (*protocol.SteerRunResponse, error) {
	b.calls.Add(1)
	b.once.Do(func() {
		close(b.entered)
		<-b.release
	})
	return &protocol.SteerRunResponse{UserItemID: "item_steer"}, nil
}

// TestKeyWaitEndsWithItsOwnRequest: the second caller for a key in flight waits
// on the holder, so its wait belongs to its own request lifetime rather than to
// the command it is queued behind.
func TestKeyWaitEndsWithItsOwnRequest(t *testing.T) {
	service := &blockingSteerService{entered: make(chan struct{}), release: make(chan struct{})}
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: newMemoryIdempotencyStore()})
	options := Options{IdempotencyKey: "steer-blocked"}
	request := protocol.SteerRunRequest{
		RunID: "run_1", ExpectedSegmentID: "seg_1",
		Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "wait"}},
	}
	first := make(chan error, 1)
	go func() {
		_, err := endpoint.Call[protocol.SteerRunRequest, *protocol.SteerRunResponse](t.Context(), RunsSteer, request, options)
		first <- err
	}()
	<-service.entered

	waiting, cancelWaiting := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancelWaiting()
	second := make(chan error, 1)
	go func() {
		_, err := endpoint.Call[protocol.SteerRunRequest, *protocol.SteerRunResponse](waiting, RunsSteer, request, options)
		second <- err
	}()
	select {
	case err := <-second:
		if err == nil {
			t.Fatal("the queued call answered while the key was still in flight")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the queued call outlived its own request")
	}

	close(service.release)
	if err := <-first; err != nil {
		t.Fatalf("first steer: %v", err)
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("SteerRun calls = %d, want 1", calls)
	}
}

// TestUnpersistedReceiptAfterACommittedOperationIsReported pins the diagnostic
// for the one failure that can cost a caller a second execution. The command's
// effect is committed and only its receipt failed, so the in-memory record is
// all that stops a retry of the same key from running it again — and that
// record lives only until the shutdown flush. A tracing span carries the reason
// only where the host installed a TracerProvider.
func TestUnpersistedReceiptAfterACommittedOperationIsReported(t *testing.T) {
	var diagnostics bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&diagnostics, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	service := &countingCancelService{}
	store := &flakyCompletionStore{Store: newMemoryIdempotencyStore()}
	store.failures.Store(1)
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: store})

	_, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](
		t.Context(), "runs.cancel",
		protocol.CancelRunRequest{RunID: "run_1"},
		Options{IdempotencyKey: "cancel-unpersisted"},
	)
	if !errors.Is(err, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("call error = %v, want idempotency_in_progress", err)
	}
	if calls := service.calls.Load(); calls != 1 {
		t.Fatalf("CancelRun calls = %d, want the command to have run once", calls)
	}
	logged := diagnostics.String()
	if !strings.Contains(logged, "unpersisted") {
		t.Fatalf("diagnostics = %q, want the unpersisted receipt reported without tracing", logged)
	}
	if !strings.Contains(logged, "runs.cancel") {
		t.Fatalf("diagnostics = %q, want the operation named", logged)
	}
}

type countingDeleteSessionService struct{ calls int }

func (s *countingDeleteSessionService) DeleteSession(context.Context, string) error {
	s.calls++
	return nil
}

func TestAcknowledgementReplaysWithoutReExecuting(t *testing.T) {
	service := &countingDeleteSessionService{}
	endpoint := mustNewEndpoint(t, service, EndpointConfig{IdempotencyStore: newMemoryIdempotencyStore()})
	for range 2 {
		if _, err := endpoint.Call[protocol.DeleteSessionRequest, struct{}](t.Context(), SessionsDelete, protocol.DeleteSessionRequest{SessionID: "ses_1"}, Options{IdempotencyKey: "delete-once"}); err != nil {
			t.Fatal(err)
		}
	}
	if service.calls != 1 {
		t.Fatalf("delete calls = %d", service.calls)
	}
}
