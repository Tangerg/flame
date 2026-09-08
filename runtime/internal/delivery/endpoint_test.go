package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

type lifetimeRuns struct {
	runUseCases
	streamStarted chan struct{}
}

func mustNewEndpoint(t *testing.T, handler *Handler, config EndpointConfig) *Endpoint {
	t.Helper()
	if config.Lifetime == nil {
		config.Lifetime = t.Context()
	}
	if config.IdempotencyStore == nil {
		config.IdempotencyStore = testsupport.NewIdempotencyStore()
	}
	endpoint, err := NewEndpoint(handler, config)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func TestEndpointRequiresProcessLifetime(t *testing.T) {
	if endpoint, err := NewEndpoint(&Handler{}, EndpointConfig{}); err == nil || endpoint != nil {
		t.Fatalf("New without lifetime = (%v, %v), want nil endpoint and non-nil error", endpoint, err)
	}
}

func TestEndpointRequiresIdempotencyStore(t *testing.T) {
	var typedNil *testsupport.IdempotencyStore
	for _, store := range []idempotency.Store{nil, typedNil} {
		endpoint, err := NewEndpoint(&Handler{}, EndpointConfig{
			Lifetime: t.Context(), IdempotencyStore: store,
		})
		if err == nil || endpoint != nil {
			t.Fatalf("New with missing store = (%v, %v), want nil endpoint and non-nil error", endpoint, err)
		}
	}
}

func TestEndpointRejectsInvalidDurableStoreNamespace(t *testing.T) {
	endpoint, err := NewEndpoint(&Handler{}, EndpointConfig{
		Lifetime:             t.Context(),
		IdempotencyNamespace: "idp_test",
	})
	if err == nil || endpoint != nil {
		t.Fatalf("New with invalid namespace = (%v, %v), want nil endpoint and non-nil error", endpoint, err)
	}
}

func TestEndpointRequiresHandler(t *testing.T) {
	endpoint, err := NewEndpoint(nil, EndpointConfig{Lifetime: t.Context(), IdempotencyStore: testsupport.NewIdempotencyStore()})
	if err == nil || endpoint != nil {
		t.Fatalf("NewEndpoint without Handler = %v, %v", endpoint, err)
	}
}

func TestEndpointRejectsMethodIncompatibleMetadataBeforeCapabilityAdmission(t *testing.T) {
	tests := []struct {
		name       string
		method     Name
		parameters any
		options    Options
	}{
		{
			name:       "query idempotency key",
			method:     RuntimeDiscover,
			parameters: struct{}{},
			options:    Options{IdempotencyKey: "query-key"},
		},
		{
			name:       "namespace without key",
			method:     RuntimeDiscover,
			parameters: struct{}{},
			options:    Options{IdempotencyNamespace: testsupport.IdempotencyNamespace},
		},
		{
			name:       "non-canonical namespace",
			method:     RunsCancel,
			parameters: protocol.CancelRunRequest{},
			options: Options{
				IdempotencyKey: "cancel-once", IdempotencyNamespace: " " + testsupport.IdempotencyNamespace,
			},
		},
		{
			name:   "runtime subscription run cursor",
			method: RuntimeSubscribe,
			parameters: protocol.RuntimeSubscribeRequest{
				Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
			},
			options: Options{AfterEventID: "evt_cursor"},
		},
		{
			name:       "run command cursor without replay key",
			method:     RunsStart,
			parameters: protocol.StartRunRequest{},
			options:    Options{AfterEventID: "evt_cursor"},
		},
		{
			name:       "run replay cursor without event framing",
			method:     RunsSubscribe,
			parameters: protocol.SubscribeRunRequest{},
			options:    Options{AfterEventID: "opaque"},
		},
		{
			name:       "run replay cursor with interior whitespace",
			method:     RunsSubscribe,
			parameters: protocol.SubscribeRunRequest{},
			options:    Options{AfterEventID: "evt_bad cursor"},
		},
		{
			name:       "run replay cursor with non-printing character",
			method:     RunsSubscribe,
			parameters: protocol.SubscribeRunRequest{},
			options:    Options{AfterEventID: "evt_bad\u200bhidden"},
		},
		{
			name:       "oversized run replay cursor",
			method:     RunsSubscribe,
			parameters: protocol.SubscribeRunRequest{},
			options: Options{AfterEventID: protocol.IDPrefixEvent + strings.Repeat(
				"x", protocol.MaximumRunEventIDCharacters,
			)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := mustNewEndpoint(t, &Handler{}, EndpointConfig{}).Invoke(
				t.Context(),
				test.method,
				test.parameters,
				test.options,
			)
			if !errors.Is(result.Failure, protocol.ErrInvalidParams) {
				t.Fatalf("failure = %v, want invalid_params", result.Failure)
			}
		})
	}
}

func TestEndpointRejectsInvalidRequestsBeforeHandlerAdmission(t *testing.T) {
	tests := []struct {
		name       string
		method     Name
		parameters any
	}{
		{name: "hook trust root", method: HooksSetTrust, parameters: protocol.SetHookTrustRequest{}},
		{name: "archive skill name", method: SkillsLibraryArchive, parameters: protocol.SkillNameRequest{}},
		{name: "restore skill name", method: SkillsLibraryRestore, parameters: protocol.SkillNameRequest{}},
		{name: "delete session id", method: SessionsDelete, parameters: protocol.DeleteSessionRequest{}},
		{
			name:   "item session scope id",
			method: ItemsList,
			parameters: protocol.ListItemsRequest{Scope: protocol.ItemListScope{
				Type: protocol.ItemScopeSession,
			}},
		},
		{
			name:   "item run scope id",
			method: ItemsList,
			parameters: protocol.ListItemsRequest{Scope: protocol.ItemListScope{
				Type: protocol.ItemScopeRun,
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := mustNewEndpoint(t, &Handler{}, EndpointConfig{}).Invoke(
				t.Context(), test.method, test.parameters, Options{},
			)
			if !errors.Is(result.Failure, protocol.ErrInvalidParams) {
				t.Fatalf("failure = %v, want invalid_params", result.Failure)
			}
		})
	}
}

func (l *lifetimeRuns) Subscribe(ctx context.Context, request runs.SubscribeRequest) (runs.Subscription, error) {
	return runs.Subscription{Record: runs.Record{SegmentID: request.SegmentID}, Events: func(func(runs.Event) bool) {
		close(l.streamStarted)
		<-ctx.Done()
	}}, nil
}

func TestEndpointLifetimeEndsStreamsAndRejectsLaterCalls(t *testing.T) {
	lifetime, stop := context.WithCancel(context.Background())
	service := &lifetimeRuns{streamStarted: make(chan struct{})}
	endpoint := mustNewEndpoint(t, &Handler{runs: service}, EndpointConfig{Lifetime: lifetime})
	result := endpoint.Invoke(t.Context(), "runs.subscribe", protocol.SubscribeRunRequest{RunID: "run_1", SegmentID: "seg_1"}, Options{})
	if result.Failure != nil || result.Events == nil {
		t.Fatalf("subscribe result = %+v", result)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range result.Events {
		}
	}()
	<-service.streamStarted
	stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Runtime lifetime cancellation did not end the stream")
	}

	result = endpoint.Invoke(t.Context(), "runs.subscribe", protocol.SubscribeRunRequest{RunID: "run_1", SegmentID: "seg_1"}, Options{})
	if !errors.Is(result.Failure, protocol.ErrInternalError) {
		t.Fatalf("post-close failure = %v, want internal_error", result.Failure)
	}
}

func TestEndpointShutdownClaimsUnstartedStreamAndJoinsItsSource(t *testing.T) {
	lifetime, stop := context.WithCancel(context.Background())
	service := &lifetimeRuns{streamStarted: make(chan struct{})}
	endpoint := mustNewEndpoint(t, &Handler{runs: service}, EndpointConfig{Lifetime: lifetime})
	result := endpoint.Invoke(t.Context(), "runs.subscribe", protocol.SubscribeRunRequest{RunID: "run_1", SegmentID: "seg_1"}, Options{})
	if result.Failure != nil || result.Events == nil {
		t.Fatalf("subscribe result = %+v", result)
	}

	stop()
	select {
	case <-service.streamStarted:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not claim the unstarted stream source")
	}
	waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Second)
	defer cancelWait()
	if err := endpoint.AwaitShutdown(waitCtx); err != nil {
		t.Fatalf("AwaitShutdown: %v", err)
	}

	// The shutdown owner already exhausted this single-consumer sequence; a
	// caller that ranges its stale handle after Close observes a clean end.
	for range result.Events {
		t.Fatal("post-shutdown stream produced an event")
	}
}

type joiningStreamRuns struct {
	runUseCases
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (j *joiningStreamRuns) Subscribe(ctx context.Context, request runs.SubscribeRequest) (runs.Subscription, error) {
	return runs.Subscription{Record: runs.Record{SegmentID: request.SegmentID}, Events: func(func(runs.Event) bool) {
		close(j.started)
		<-ctx.Done()
		close(j.canceled)
		<-j.release
	}}, nil
}

func TestEndpointShutdownWaitsForStartedStreamSourceToReturn(t *testing.T) {
	service := &joiningStreamRuns{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	endpoint := mustNewEndpoint(t, &Handler{runs: service}, EndpointConfig{})
	result := endpoint.Invoke(t.Context(), "runs.subscribe", protocol.SubscribeRunRequest{RunID: "run_1", SegmentID: "seg_1"}, Options{})
	if result.Failure != nil || result.Events == nil {
		t.Fatalf("subscribe result = %+v", result)
	}
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		for range result.Events {
		}
	}()
	select {
	case <-service.started:
	case <-time.After(time.Second):
		t.Fatal("stream source did not start")
	}

	endpoint.BeginShutdown()
	select {
	case <-service.canceled:
	case <-time.After(time.Second):
		t.Fatal("stream source did not observe shutdown")
	}
	shortCtx, cancelShort := context.WithTimeout(t.Context(), 20*time.Millisecond)
	if err := endpoint.AwaitShutdown(shortCtx); !errors.Is(err, context.DeadlineExceeded) {
		cancelShort()
		t.Fatalf("AwaitShutdown before source return = %v, want deadline exceeded", err)
	}
	cancelShort()

	close(service.release)
	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("released stream source did not return")
	}
	waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Second)
	defer cancelWait()
	if err := endpoint.AwaitShutdown(waitCtx); err != nil {
		t.Fatalf("AwaitShutdown after source return: %v", err)
	}
}
