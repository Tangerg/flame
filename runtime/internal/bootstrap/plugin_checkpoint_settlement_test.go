package bootstrap

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	framehost "github.com/Tangerg/frame/host"
)

func TestPluginCheckpointSettlesWhenItsCallerCancels(t *testing.T) {
	cfg := poMCPConfig(t)
	var interrupted atomic.Bool
	commit := cfg.stores.CommitPluginReferences
	cfg.stores.CommitPluginReferences = func(ctx context.Context, host framehost.Host, id string, checkpoint runs.ExecutorCheckpoint, apply func(context.Context) error) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		return commit(ctx, host, id, checkpoint, func(tx context.Context) error {
			if interrupted.CompareAndSwap(false, true) {
				cancel()
			}
			return apply(tx)
		})
	}
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "memory"))
	id := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "memory"}
	r.poMCPTools(id, memoryToolsReady)
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Finish an admitted checkpoint"}, "checkpoint-session").(*protocol.Session)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	result := r.endpoint.Invoke(ctx, delivery.RunsStart, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Finish this Run."}},
	}, protocol.CallMetadata{IdempotencyKey: "checkpoint-run", RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}})
	if result.Failure != nil {
		t.Fatalf("Run opening failed: %+v", result.Failure)
	}
	started := result.Value.(*protocol.StartRunResponse)
	for _, err := range result.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !interrupted.Load() {
		t.Fatal("the installed Run did not reach a reserved checkpoint write")
	}
	finished := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.RunID}, "").(*protocol.RunRef)
	if finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("cancellation interrupted an already reserved checkpoint: %+v", finished.Outcome)
	}
	r.close()
}
