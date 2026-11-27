package bootstrap

import (
	"context"
	"database/sql/driver"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chatclient"
	modernsqlite "modernc.org/sqlite"
)

func TestGoalPausesWhenCommittedResourceRetirementFails(t *testing.T) {
	var attempts atomic.Int32
	retried := make(chan struct{})
	if err := modernsqlite.RegisterScalarFunction("test_count_goal_retirement", 0,
		func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			if attempts.Add(1) == 2 {
				close(retried)
			}
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	model := newLifecycleModel()
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.modelProviders = testChatResolver(client)
	runtime := poMCPOpen(t, cfg)
	poMCPActivate(runtime, filepath.Join(firstPartyPackages, "goals"))
	ctx, stop := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
	defer stop()
	session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "Session with committed cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := runtime.api.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: session.ID})
	if err != nil || archive.Artifact == nil {
		t.Fatalf("export Session before replacement: %v", err)
	}
	questionCrashDatabase(t, cfg.stores.DataDirectory, `CREATE TRIGGER refuse_goal_retirement_completion
		BEFORE DELETE ON committed_session_resource_retirements
		BEGIN
			SELECT test_count_goal_retirement();
			SELECT RAISE(ABORT, 'test refused retirement completion');
		END`)
	if _, err := runtime.api.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: *archive.Artifact}); err != nil {
		t.Fatalf("committed replacement failed: %v", err)
	}
	id := testsupport.MustSessionID(session.ID)
	original, pending, err := cfg.stores.ResourceRetirements.Lookup(ctx, id)
	if err != nil || !pending || attempts.Swap(0) != 1 {
		t.Fatalf("original cleanup did not leave a retirement: pending=%t, %v", pending, err)
	}
	started, err := runtime.api.StartGoal(ctx, protocol.StartGoalRequest{
		SessionID: session.ID, Objective: "Continue after cleanup is available.",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRetirementPause(t, ctx, runtime, session.ID, retried)
	current, pending, err := cfg.stores.ResourceRetirements.Lookup(ctx, id)
	if err != nil || !pending || current != original || attempts.Load() != 1 {
		t.Fatalf("failed admission changed the original retirement: attempts=%d, pending=%t, %v", attempts.Load(), pending, err)
	}
	page, err := runtime.api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID})
	if err != nil || len(page.Data) != 0 {
		t.Fatalf("failed admission opened a Run: %+v, %v", page, err)
	}

	questionCrashDatabase(t, cfg.stores.DataDirectory, `DROP TRIGGER refuse_goal_retirement_completion`)
	if _, err := runtime.api.ResumeGoal(ctx, started.GoalTarget); err != nil {
		t.Fatal(err)
	}
	waitForSignal(t, model.firstCallStarted, "the resumed Goal's original retirement to settle before dispatch")
	if _, pending, err := cfg.stores.ResourceRetirements.Lookup(ctx, id); err != nil || pending {
		t.Fatalf("resumed Goal crossed a pending retirement: pending=%t, %v", pending, err)
	}
	if err := runtime.api.StopGoal(ctx, started.GoalTarget); err != nil {
		t.Fatal(err)
	}
}

func waitForRetirementPause(t *testing.T, ctx context.Context, runtime *poMCPRuntime, sessionID string, retried <-chan struct{}) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-retried:
			t.Fatal("Goal retried the failed retirement instead of pausing")
		case <-ctx.Done():
			t.Fatalf("Goal did not settle its failed admission: %v", ctx.Err())
		case <-ticker.C:
			current, err := runtime.api.GetGoal(ctx, protocol.GetGoalRequest{SessionID: sessionID})
			if err != nil || current == nil {
				t.Fatalf("read Goal after failed admission: %+v, %v", current, err)
			}
			if current.Status == protocol.GoalPaused && current.Reason != nil && current.Reason.Code == protocol.GoalReasonRunStartFailed {
				return
			}
		}
	}
}
