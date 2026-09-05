package bootstrap

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/infra/process/teardown"
	sqlitestore "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestNewRequiresRuntimeDependencies(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{
			name: "stores",
			edit: func(cfg *Config) { cfg.Stores = nil },
			want: "runtime: Stores is required",
		},
		{
			name: "user home",
			edit: func(cfg *Config) {
				cfg.UserHome = ""
			},
			want: "runtime: UserHome is required",
		},
		{
			name: "relative user home",
			edit: func(cfg *Config) {
				cfg.UserHome = "relative-home"
			},
			want: "runtime: UserHome must be absolute",
		},
		{
			name: "default workspace path",
			edit: func(cfg *Config) {
				cfg.DefaultWorkspacePath = ""
			},
			want: "runtime: DefaultWorkspacePath is required",
		},
		{
			name: "relative default workspace path",
			edit: func(cfg *Config) {
				cfg.DefaultWorkspacePath = "relative-workspace"
			},
			want: "runtime: DefaultWorkspacePath must be absolute",
		},
		{
			name: "relative skills user directory",
			edit: func(cfg *Config) {
				cfg.SkillsUserDir = "relative-skills"
			},
			want: "runtime: SkillsUserDir must be absolute when set",
		},
		{
			name: "relative sandbox directory",
			edit: func(cfg *Config) {
				cfg.SandboxDir = "relative-sandbox"
			},
			want: "runtime: SandboxDir must be absolute when set",
		},
		{
			name: "relative sandbox read-only path",
			edit: func(cfg *Config) {
				cfg.SandboxReadOnlyPaths = []string{"relative-read-only"}
			},
			want: "runtime: SandboxReadOnlyPaths[0] must be absolute when set",
		},
		{
			name: "relative recipes global directory",
			edit: func(cfg *Config) {
				cfg.RecipesGlobalDir = "relative-recipes"
			},
			want: "runtime: RecipesGlobalDir must be absolute when set",
		},
		{
			name: "relative checkpoint directory",
			edit: func(cfg *Config) {
				cfg.CheckpointDir = "relative-checkpoints"
			},
			want: "runtime: CheckpointDir must be absolute when set",
		},
		{
			name: "chat resolver",
			edit: func(cfg *Config) {
				cfg.ChatResolver = nil
			},
			want: "runtime: ChatResolver is required",
		},
		{
			name: "provider registry",
			edit: func(cfg *Config) {
				cfg.ProviderRegistry = nil
			},
			want: "runtime: ProviderRegistry is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := runtimeConfigWithRequiredDeps(t)
			tt.edit(&cfg)

			_, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.Resources), buildToolEnvironment)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("assemble error = %v, want containing %q", err, tt.want)
			}

		})
	}
}

func TestAssemblyFailureReclaimsToolsAndOwnedResources(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	buildErr := errors.New("complete tool environment rejected")
	var (
		toolClosed     atomic.Int32
		resourceClosed atomic.Int32
	)
	cfg.Resources = []TerminalResource{closerFunc(func() error {
		resourceClosed.Add(1)
		return nil
	})}

	buildTools := func(
		ctx context.Context,
		deps toolEnvironmentDependencies,
	) (toolEnvironment, error) {
		toolRuntime, err := buildToolEnvironment(ctx, deps)
		if err != nil {
			return toolEnvironment{}, err
		}
		toolRuntime.closers = append(toolRuntime.closers, teardown.Terminal(func(context.Context) error {
			toolClosed.Add(1)
			return nil
		}))
		return toolRuntime, buildErr
	}
	host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.Resources), buildTools)
	if !errors.Is(err, buildErr) {
		t.Fatalf("assemble error = %v, want complete tool-environment failure", err)
	}
	if host != nil {
		t.Fatal("failed Build returned an Instance")
	}
	if got := toolClosed.Load(); got != 1 {
		t.Fatalf("tool closer calls = %d, want 1", got)
	}
	if got := resourceClosed.Load(); got != 1 {
		t.Fatalf("owned resource closer calls = %d, want 1", got)
	}
}

func TestAssemblyRejectsInvalidDefaultModelBeforeStartupReconciliation(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	db, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "tool-results.db"))
	if err != nil {
		t.Fatalf("open tool-result store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlitestore.NewToolResultStore(db)
	id := toolresult.NewID()
	if err := store.Stage(t.Context(), toolresult.Stage{
		ID: id, SessionID: "ses_staged", ToolName: "shell", Body: "unbound",
	}); err != nil {
		t.Fatalf("stage tool result: %v", err)
	}
	cfg.Stores.ToolResults = store
	cfg.Provider = ""

	_, err = assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.Resources), buildToolEnvironment)
	if err == nil || !strings.Contains(err.Error(), "default model selection") {
		t.Fatalf("assembly error = %v, want invalid default model selection", err)
	}
	if _, found, fetchErr := store.Fetch(t.Context(), "ses_staged", id); fetchErr != nil || !found {
		t.Fatalf("staged tool result after rejected config = (found %v, err %v), want (true, nil)", found, fetchErr)
	}

}

func TestAssemblyBuilderFailureReclaimsReturnedAcquisitions(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	buildErr := errors.New("tool environment failed")
	var closed atomic.Int32

	buildTools := func(
		context.Context,
		toolEnvironmentDependencies,
	) (toolEnvironment, error) {
		return toolEnvironment{
			closers: []*teardown.Step{teardown.Terminal(func(context.Context) error {
				closed.Add(1)
				return nil
			})},
		}, buildErr
	}
	host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.Resources), buildTools)
	if !errors.Is(err, buildErr) {
		t.Fatalf("assemble error = %v, want build failure", err)
	}
	if host != nil {
		t.Fatal("successful rollback returned an Instance owner")
	}
	if got := closed.Load(); got != 1 {
		t.Fatalf("returned acquisition close calls = %d, want 1", got)
	}
}

func TestAssemblyFailureRollbackContinuesAfterCloseTimeout(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	// Fail after tool acquisition. OpenInstance receives no Instance on this path and
	// can make only its bounded rollback calls, so the graph itself must retain
	// ownership when a terminal closer finishes after both callers time out.
	buildErr := errors.New("complete tool environment rejected")
	resourceClosed := make(chan struct{})
	cfg.Resources = []TerminalResource{closerFunc(func() error {
		close(resourceClosed)
		return nil
	})}
	closerStarted := make(chan struct{})
	releaseCloser := make(chan struct{})
	buildTools := func(
		ctx context.Context,
		deps toolEnvironmentDependencies,
	) (toolEnvironment, error) {
		toolRuntime, err := buildToolEnvironment(ctx, deps)
		if err != nil {
			return toolEnvironment{}, err
		}
		toolRuntime.closers = append(toolRuntime.closers, teardown.Terminal(func(context.Context) error {
			close(closerStarted)
			<-releaseCloser
			return nil
		}))
		return toolRuntime, buildErr
	}
	lifetime := newRuntimeLifetime(t.Context(), cfg.Resources)
	lifetime.shutdownWait = testShutdownWait(t, time.Millisecond)

	failedInstance, err := assemble(t.Context(), cfg, lifetime, buildTools)
	if failedInstance != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("failed Build = (%v, %v), want nil Instance and bounded rollback timeout", failedInstance, err)
	}
	<-closerStarted
	if err := closeRuntimeLifetime(lifetime); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded shutdown rollback = %v, want deadline exceeded", err)
	}

	close(releaseCloser)
	select {
	case <-resourceClosed:
	case <-time.After(time.Second):
		t.Fatal("failed construction lost its dependent resource owner after rollback timeout")
	}
}

func TestAssemblyDirectToolsDoNotDependOnAgentResolver(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	var toolClosed atomic.Int32

	buildTools := func(
		ctx context.Context,
		deps toolEnvironmentDependencies,
	) (toolEnvironment, error) {
		toolRuntime, err := buildToolEnvironment(ctx, deps)
		if err != nil {
			return toolEnvironment{}, err
		}
		toolRuntime.closers = append(toolRuntime.closers, teardown.Terminal(func(context.Context) error {
			toolClosed.Add(1)
			return nil
		}))
		// The agent resolver is intentionally absent. Direct client-invoked
		// diagnostics have a separate fixed catalog and must not inherit the
		// model-driven Run's capability catalog.
		toolRuntime.tools.Resolver = nil
		return toolRuntime, nil
	}
	host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.Resources), buildTools)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := host.Close(); err != nil {
		t.Fatalf("close host: %v", err)
	}
	if got := toolClosed.Load(); got != 1 {
		t.Fatalf("tool closer calls = %d, want 1", got)
	}
}

func runtimeConfigWithRequiredDeps(t *testing.T) Config {
	t.Helper()

	client, err := chatclient.New(newReplyStub("ok"), chatclient.Config{})
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}

	workspace := t.TempDir()
	stores, err := persistence.Open(t.Context(), persistence.Config{
		DataDirectory: t.TempDir(), DefaultWorkspacePath: workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	return Config{
		Stores:   stores,
		Provider: "anthropic", Model: "claude-test",
		ApprovalMode: approval.ModeSafe,
		UserHome:     t.TempDir(), DefaultWorkspacePath: workspace,
		ChatResolver:     testChatResolver(client),
		BuildID:          "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		ProviderRegistry: stores.Providers,
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
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: transcript.QuestionText}}}
	value := testsupport.MustRestoreSession(session.Snapshot{
		ID: sessionID, Workspace: testsupport.MustWorkspace(t.TempDir()), StartedAt: createdAt, UpdatedAt: createdAt,
	})
	if err := cfg.Stores.Sessions.Insert(ctx, value); err != nil {
		t.Fatalf("insert Session: %v", err)
	}
	profile := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	}
	if err := cfg.Stores.Runs.Admit(ctx, run.Draft{
		RunID: runID, SessionID: sessionID, SegmentID: "seg_open",
		Capabilities: profile, ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := cfg.Stores.Runs.Suspend(ctx, testsupport.MustRestoreRun(run.Snapshot{SessionID: sessionID, ID: runID, State: run.Waiting,
		Capabilities: profile,
		CreatedAt:    createdAt, MessageMark: run.UnknownMessageMark}),
		"seg_open", runtimeidentity.CommitID{},
	); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := cfg.Stores.Transcript.AppendItem(ctx, testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_park", RunID: runID, SessionID: sessionID,
		Kind:     transcript.QuestionItem,
		Question: question, OccurredAt: parkedAt,
	})); err != nil {
		t.Fatalf("put transcript item: %v", err)
	}
	if err := cfg.Stores.Interrupts.Open(ctx, bootstrapPending(
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
	if err := cfg.Stores.ExecutorCheckpoints.SaveCheckpoint(ctx, bootstrapCheckpoint(memberID, sessionID)); err != nil {
		t.Fatalf("save executor checkpoint: %v", err)
	}

	host, err := assemble(ctx, cfg, newRuntimeLifetime(t.Context(), cfg.Resources), buildToolEnvironment)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })

	if pending, listErr := cfg.Stores.Interrupts.List(ctx, sessionID); listErr != nil || len(pending) != 0 {
		t.Fatalf("pending after assemble = (%+v, %v), want none", pending, listErr)
	}
	if _, loadCheckpointErr := cfg.Stores.ExecutorCheckpoints.LoadCheckpoint(ctx, memberID); !errors.Is(loadCheckpointErr, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("executor checkpoint after assemble = %v, want not found", loadCheckpointErr)
	}
	runs, err := cfg.Stores.Runs.ListRuns(ctx, sessionID)
	failure, failed := run.Failure{}, false
	if len(runs) == 1 {
		failure, failed = runs[0].Failure()
	}
	if err != nil || len(runs) != 1 || !failed || failure.Kind != run.FailureLost {
		t.Fatalf("runs after assemble = (%+v, %v), want run_lost", runs, err)
	}
}
