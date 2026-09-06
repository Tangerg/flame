package bootstrap

import (
	"errors"
	"testing"

	ownershipadapter "github.com/Tangerg/flame/runtime/internal/adapter/ownership"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestAssemblySharesDataDirectoryOwnershipForRunsGoalsAndRecovery(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	host, api := buildProtocolRuntime(t, cfg, cfg.DefaultWorkspacePath)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Errorf("close Runtime: %v", err)
		}
	})
	leases, err := ownershipadapter.New(cfg.Stores.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: cfg.DefaultWorkspacePath}, Title: "shared ownership",
	})
	if err != nil {
		t.Fatal(err)
	}

	sessionLease, acquired, err := leases.TrySession(session.ID)
	if err != nil || !acquired {
		t.Fatalf("acquire Session lease: acquired=%t err=%v", acquired, err)
	}
	t.Cleanup(sessionLease.Release)
	request := protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "check admission"}},
	}
	if started, _, err := api.StartRun(ctx, request); started != nil || !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("start under Session lease = (%+v, %v), want busy", started, err)
	}
	sessionLease.Release()

	goalLease, acquired, err := leases.TryGoalDrive(session.ID)
	if err != nil || !acquired {
		t.Fatalf("acquire Goal lease: acquired=%t err=%v", acquired, err)
	}
	t.Cleanup(goalLease.Release)
	if started, err := api.StartGoal(ctx, protocol.StartGoalRequest{
		SessionID: session.ID, Objective: "check Goal ownership", Provider: "anthropic", Model: "claude-test",
	}); started != nil || !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("start under Goal lease = (%+v, %v), want busy", started, err)
	}
	if current, err := api.GetGoal(ctx, protocol.GoalRequest{SessionID: session.ID}); err != nil || current != nil {
		t.Fatalf("Goal after denied drive = (%+v, %v), want absent", current, err)
	}
	goalLease.Release()

	recoveryLease, acquired, err := leases.TryRecoverySweep()
	if err != nil || !acquired {
		t.Fatalf("acquire recovery lease: acquired=%t err=%v", acquired, err)
	}
	t.Cleanup(recoveryLease.Release)
	if acquired, err := host.application.workers.recovery.Reconcile(t.Context()); err != nil || acquired {
		t.Fatalf("recovery under held lease = (%t, %v), want contention", acquired, err)
	}
	recoveryLease.Release()
	if acquired, err := host.application.workers.recovery.Reconcile(t.Context()); err != nil || !acquired {
		t.Fatalf("recovery after release = (%t, %v), want acquired", acquired, err)
	}

	started, events, err := api.StartRun(ctx, request)
	if err != nil {
		t.Fatalf("start after release: %v", err)
	}
	waitForRunEvents(t, collectRunEvents(events), "Run after ownership release")
	completed, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || completed == nil || completed.Status != protocol.RunStatusFinished ||
		completed.Outcome == nil || completed.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Run after ownership release = (%+v, %v)", completed, err)
	}
}
