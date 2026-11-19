package bootstrap

import (
	"context"
	"database/sql/driver"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
	modernsqlite "modernc.org/sqlite"
)

const (
	modelAttemptCrashPathEnv = "FLAME_TEST_MODEL_ATTEMPT_CRASH_PATH"
	modelAttemptCrashPrompt  = "Exit before obtaining a physical model endpoint."
)

func TestOpenModelAttemptCrashHelper(t *testing.T) {
	target := os.Getenv(modelAttemptCrashPathEnv)
	if target == "" {
		return
	}
	if err := modernsqlite.RegisterScalarFunction("test_exit_before_model_endpoint", 0,
		func(_ *modernsqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("FLAME_HOME")
	host, api := openProtocolRuntime(t, newReplyStub("dispatch must not be reached"))
	defer host.Close()
	questionCrashDatabase(t, home, `CREATE TRIGGER crash_before_model_endpoint BEFORE INSERT ON model_invocations
		WHEN NEW.endpoint_json IS NOT NULL AND NEW.run_id IS NOT NULL
		BEGIN SELECT test_exit_before_model_endpoint(); END`)
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "logical model attempt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(session.ID), 0o600); err != nil {
		t.Fatal(err)
	}
	_, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: modelAttemptCrashPrompt}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range events {
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("did not exit after the logical invocation opened")
}

func TestModelAttemptWithoutEndpointRecoversAfterProcessExit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	cutPath := filepath.Join(t.TempDir(), "session")
	crashCtx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	child := exec.CommandContext(crashCtx, os.Args[0], "-test.run=^TestOpenModelAttemptCrashHelper$")
	child.Env = append(os.Environ(), modelAttemptCrashPathEnv+"="+cutPath)
	out, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("the model did not reach the intended hard exit: %v\n%s", err, out)
	}
	recordedSession, err := os.ReadFile(cutPath)
	if err != nil {
		t.Fatal(err)
	}
	sessionID := string(recordedSession)
	ctx := protocolLifecycleContext(t.Context())
	questionCrashDatabase(t, home, "DROP TRIGGER crash_before_model_endpoint")
	runID, attempt := readAbandonedModelAttempt(t, ctx, home, sessionID)

	var modelCalls atomic.Int32
	reply := newReplyStub("only a new Run may generate")
	model := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		modelCalls.Add(1)
		return reply.Call(ctx, request)
	})
	host, api, stores := openProtocolRuntimeWithStores(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	recovered, err := api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: sessionID})
	if err != nil || len(recovered.Data) != 1 {
		t.Fatalf("recovery changed the original Run set: %+v, %v", recovered, err)
	}
	root := recovered.Data[0]
	if root.ID != runID || root.Outcome == nil || root.Outcome.Type != protocol.OutcomeLost {
		t.Fatalf("abandoned Run was not recovered as Lost: %+v", root)
	}
	calls, err := api.ListModelInvocations(ctx, protocol.ListModelInvocationsRequest{RunID: runID})
	if err != nil || len(calls.Data) != 1 {
		t.Fatalf("recovery changed the original model invocation set: %+v, %v", calls, err)
	}
	closed := calls.Data[0]
	if closed.CallID != attempt.CallID || closed.SegmentID != attempt.SegmentID || !closed.StartedAt.Equal(attempt.StartedAt) ||
		closed.State != protocol.ModelInvocationUnknown || closed.SettledAt.Before(closed.StartedAt) || closed.Usage != nil {
		t.Fatalf("original logical attempt was not closed as unknown: %+v", closed)
	}
	endpoints, err := stores.ModelInvocations.ListSessionEndpoints(ctx, sessionID)
	if err != nil || len(endpoints) != 0 {
		t.Fatalf("recovery invented a physical endpoint: count=%d, %v", len(endpoints), err)
	}
	if issued := modelCalls.Load(); issued != 0 {
		t.Fatalf("recovery dispatched %d model calls, want zero", issued)
	}
	opened, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: sessionID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "continue"}},
	})
	if err != nil {
		t.Fatalf("the recovered Run retained Session admission: %v", err)
	}
	waitForRunEvents(t, collectRunEvents(events), "a new Run after model-attempt recovery")
	final, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: opened.RunID})
	if err != nil || final.Outcome == nil || final.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("new Run after recovery: %+v, %v", final, err)
	}
	if issued := modelCalls.Load(); issued != 1 {
		t.Fatalf("only the new Run should dispatch: got %d model calls", issued)
	}
}

func readAbandonedModelAttempt(t *testing.T, ctx context.Context, home, sessionID string) (string, runs.ModelInvocationCommit) {
	t.Helper()
	stores, err := persistence.Open(ctx, persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stores.Close(); err != nil {
			t.Error(err)
		}
	}()
	tree, err := stores.Runs.ListRuns(ctx, sessionID)
	if err != nil || len(tree) != 1 || tree[0].State().IsTerminal() {
		t.Fatalf("pre-recovery Run: %+v, %v", tree, err)
	}
	attempts, err := stores.ModelInvocations.PageModelInvocations(ctx, tree[0].ID(), 0, "", 2)
	if err != nil || len(attempts) != 1 || attempts[0].State != modelref.InvocationStarted {
		t.Fatalf("pre-recovery logical invocation: %+v, %v", attempts, err)
	}
	attempt := attempts[0]
	if _, found, err := stores.ModelInvocations.EndpointForCaller(ctx, attempt.CallID); err != nil || found {
		t.Fatalf("crash cut already has a physical endpoint: found=%t, %v", found, err)
	}
	return tree[0].ID(), attempt
}
