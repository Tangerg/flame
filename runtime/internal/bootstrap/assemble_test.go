package bootstrap

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/frame/process/runner"
	"github.com/Tangerg/scope/core/chatclient"
)

func runtimeConfigWithRequiredDeps(t *testing.T) assemblyConfig {
	t.Helper()

	client, err := chatclient.New(newReplyStub("ok"))
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}

	workspace := t.TempDir()
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: testsupport.TempDirectory(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	return assemblyConfig{
		processRunner:    testProcessRunner(t),
		stores:           stores,
		defaultSelection: testsupport.MustModelSelection("anthropic", "claude-test"),
		approvalMode:     approval.ModeSafe,
		userHome:         t.TempDir(), defaultWorkspacePath: workspace,
		skillsUserDir:    t.TempDir(),
		modelProviders:   testChatResolver(client),
		buildID:          "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		providerRegistry: stores.Providers,
	}
}

func TestAssemblyRecoversParkedRunWithIncompatibleDeployment(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	ctx := t.Context()
	const (
		runID     = "run_park"
		sessionID = "ses_park"
		memberID  = "member_park"
	)
	createdAt := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	parkedAt := createdAt.Add(time.Second)
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: questioncontract.Text}}}
	value := testsupport.MustRestoreSession(session.Snapshot{
		ID: sessionID, Workspace: testsupport.MustWorkspace(t.TempDir()), CreatedAt: createdAt, UpdatedAt: createdAt,
	})
	if err := cfg.stores.Sessions.Insert(ctx, value); err != nil {
		t.Fatalf("insert Session: %v", err)
	}
	profile := run.Capabilities{
		InterruptTypes: []run.InterruptType{run.InterruptQuestion},
	}
	if err := cfg.stores.Runs.Admit(ctx, run.Draft{
		RunID: runID, SessionID: sessionID, SegmentID: "seg_open",
		Capabilities: profile, ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := cfg.stores.Runs.Suspend(ctx, testsupport.MustRestoreRun(run.Snapshot{SessionID: sessionID, ID: runID, State: run.Waiting,
		Capabilities: profile,
		CreatedAt:    createdAt, MessageMark: run.UnknownMessageMark()}),
		"seg_open", runs.CommitID{},
	); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := cfg.stores.Transcript.AppendItem(ctx, testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_park", RunID: runID, SessionID: sessionID,
		Kind:     transcript.QuestionItem,
		Question: question, OccurredAt: parkedAt,
	})); err != nil {
		t.Fatalf("put transcript item: %v", err)
	}
	if err := cfg.stores.Interrupts.Open(ctx, bootstrapPending(
		runID,
		sessionID,
		memberID,
		"item_park",
		createdAt,
		parkedAt,
		parkedAt,
	)); err != nil {
		t.Fatalf("open interrupt: %v", err)
	}
	if err := cfg.stores.ExecutorCheckpoints.SaveCheckpoint(ctx, bootstrapCheckpoint(memberID, sessionID)); err != nil {
		t.Fatalf("save executor checkpoint: %v", err)
	}

	host, err := assemble(ctx, cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildToolEnvironment)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })

	// Uncertain-execution revival is the post-callback-RPC recovery phase; the
	// production owner runs it after publishing delivery. This parked Run has no
	// plugin-backed effect, so it revives without a live plugin session.
	if err := host.application.recoverUncertainExecutions(ctx); err != nil {
		t.Fatalf("recover uncertain executions: %v", err)
	}

	if pending, listErr := cfg.stores.Interrupts.List(ctx, sessionID); listErr != nil || len(pending) != 0 {
		t.Fatalf("pending after assemble = (%+v, %v), want none", pending, listErr)
	}
	if _, loadCheckpointErr := cfg.stores.ExecutorCheckpoints.LoadCheckpoint(ctx, memberID); !errors.Is(loadCheckpointErr, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("executor checkpoint after assemble = %v, want not found", loadCheckpointErr)
	}
	runs, err := cfg.stores.Runs.ListRuns(ctx, sessionID)
	lost := false
	if len(runs) == 1 {
		_, lost = runs[0].Loss()
	}
	if err != nil || len(runs) != 1 || !lost {
		t.Fatalf("runs after assemble = (%+v, %v), want run_lost", runs, err)
	}
}

func testProcessRunner(t *testing.T) *runner.Daemon {
	t.Helper()
	// The operating system limits Unix socket paths independently of test names.
	directory, err := os.MkdirTemp("", "flame-runner-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	value, err := runner.NewDaemon(runner.DaemonConfig{Executable: testsupport.RunnerExecutable(t), Directory: directory, StartTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := value.Close(ctx); err != nil {
			t.Errorf("close test process runner: %v", err)
		}
	})
	return value
}
