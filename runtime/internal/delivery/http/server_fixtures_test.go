package http_test

import (
	"bytes"
	"context"
	netHTTP "net/http"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

const testRuntimeInstanceID = testsupport.RuntimeInstanceID

type fakeRuntime struct {
	canceledRuns []string
}

func (f *fakeRuntime) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{
		ProtocolVersion: protocol.Version,
		ServerInfo: protocol.ServerInfo{
			Name: "flame-test", Version: "0.0.0", InstanceID: testRuntimeInstanceID,
			DefaultWorkspace: protocol.WorkspaceRef{Path: "/workspace"}, Home: "/home",
		},
		Capabilities: validTestCapabilities(),
	}, nil
}

func (f *fakeRuntime) CancelRun(_ context.Context, in protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
	f.canceledRuns = append(f.canceledRuns, in.RunID)
	outcome := protocol.RunOutcome{Type: protocol.OutcomeCanceled}
	finishedAt := time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC)
	return &protocol.CancelRunResponse{
		Type: protocol.CancelRunRoot,
		Run: protocol.RunRef{RunSummary: protocol.RunSummary{
			ID: in.RunID, SessionID: "ses_test", Provider: "mock", Model: "balanced",
			Status: protocol.RunStatusFinished, Outcome: &outcome,
			CreatedAt: finishedAt.Add(-time.Second), FinishedAt: finishedAt,
		}},
	}, nil
}

func validTestCapabilities() protocol.ServerCapabilities {
	return protocol.ServerCapabilities{
		RunEvents:        []protocol.StreamEventType{},
		RuntimeTopics:    []protocol.RuntimeTopic{},
		StreamingMethods: []string{},
		Features:         map[string]protocol.FeatureCapability{},
		Limits: protocol.RuntimeLimits{
			Idempotency: protocol.IdempotencyLimits{RetentionSeconds: 1, Namespace: testsupport.IdempotencyNamespace},
			RunReplay: protocol.RunReplayLimits{
				Scope: protocol.ReplayScopeRuntimeInstanceRootSegment, MaxEvents: 1, MaxBytes: 1,
			},
			MCPAuthorizationAttempts: protocol.MCPAuthorizationAttemptLimits{RetentionSeconds: 1},
			RuntimeSubscription:      protocol.SubscriptionLimits{MaxTopics: 1, MaxWatches: 1, MaxPaths: 1, MaxDirectoryEntries: 1, MaxFileBytes: 1},
		},
	}
}

func newTestEndpoint(t *testing.T, target any, config delivery.EndpointConfig) *delivery.Endpoint {
	t.Helper()
	config.Lifetime = t.Context()
	if config.IdempotencyStore == nil {
		stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: t.TempDir(), ValidateDirectory: persistencefixture.UnleasedDirectory})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := stores.Close(); err != nil {
				t.Error(err)
			}
		})
		config.IdempotencyStore = stores.Idempotency
		config.IdempotencyNamespace = stores.IdempotencyNamespace.String()
		config.ValidateStorage = stores.ValidateDirectory
	}
	endpoint, err := delivery.NewEndpoint(target, config)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func readBody(r *netHTTP.Response) string {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r.Body)
	return buf.String()
}
