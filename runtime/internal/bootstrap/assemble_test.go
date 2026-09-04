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
			name: "conversation store",
			edit: func(cfg *Config) {
				cfg.ConversationStore = nil
			},
			want: "runtime: ConversationStore is required",
		},
		{
			name: "provider registry",
			edit: func(cfg *Config) {
				cfg.ProviderRegistry = nil
			},
			want: "runtime: ProviderRegistry is required",
		},
		{
			name: "mcp registry",
			edit: func(cfg *Config) {
				cfg.MCPRegistry = nil
			},
			want: "runtime: MCPRegistry is required",
		},
		{
			name: "mcp oauth sessions",
			edit: func(cfg *Config) {
				cfg.MCPOAuthSessions = nil
			},
			want: "runtime: MCPOAuthSessions is required",
		},
		{
			name: "session store",
			edit: func(cfg *Config) {
				cfg.SessionStore = nil
			},
			want: "runtime: SessionStore is required",
		},
		{
			name: "interrupt store",
			edit: func(cfg *Config) {
				cfg.InterruptStore = nil
			},
			want: "runtime: InterruptStore is required",
		},
		{
			name: "transcript store",
			edit: func(cfg *Config) {
				cfg.TranscriptStore = nil
			},
			want: "runtime: TranscriptStore is required",
		},
		{
			name: "run store",
			edit: func(cfg *Config) {
				cfg.RunStore = nil
			},
			want: "runtime: RunStore is required",
		},
		{
			name: "executor checkpoint store",
			edit: func(cfg *Config) {
				cfg.ExecutorCheckpoints = nil
			},
			want: "runtime: ExecutorCheckpoints is required",
		},
		{
			name: "transactor",
			edit: func(cfg *Config) {
				cfg.Transactor = nil
			},
			want: "runtime: Transactor is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := runtimeConfigWithRequiredDeps(t)
			tt.edit(&cfg)

			assembly := NewAssembly(t.Context(), cfg)
			_, err := BuildAssembly(t.Context(), assembly)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Assembly.Build error = %v, want containing %q", err, tt.want)
			}
			_ = CloseAssembly(assembly)
		})
	}
}

func TestAssemblyCloseBeforeBuildReleasesResourcesAndConsumesBuilder(t *testing.T) {
	var closed atomic.Int32
	assembly := NewAssembly(t.Context(), Config{
		Resources: []TerminalResource{closerFunc(func() error {
			closed.Add(1)
			return nil
		})},
	})

	if err := CloseAssembly(assembly); err != nil {
		t.Fatalf("Assembly.Close: %v", err)
	}
	if got := closed.Load(); got != 1 {
		t.Fatalf("owned resource closer calls = %d, want 1", got)
	}
	if host, err := BuildAssembly(t.Context(), assembly); err == nil || host != nil {
		t.Fatalf("Build after Close = (%v, %v), want consumed Assembly", host, err)
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

	assembly := newAssembly(t.Context(), cfg, func(
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
	})
	host, err := BuildAssembly(t.Context(), assembly)
	if !errors.Is(err, buildErr) {
		t.Fatalf("Assembly.Build error = %v, want complete tool-environment failure", err)
	}
	if host != nil {
		t.Fatal("failed Build returned a Host")
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
	cfg.ToolResultStore = store
	cfg.Provider = ""

	assembly := NewAssembly(t.Context(), cfg)
	_, err = BuildAssembly(t.Context(), assembly)
	if err == nil || !strings.Contains(err.Error(), "default model selection") {
		t.Fatalf("BuildAssembly error = %v, want invalid default model selection", err)
	}
	if _, found, fetchErr := store.Fetch(t.Context(), "ses_staged", id); fetchErr != nil || !found {
		t.Fatalf("staged tool result after rejected config = (found %v, err %v), want (true, nil)", found, fetchErr)
	}
	_ = CloseAssembly(assembly)
}

func TestAssemblyBuilderFailureReclaimsReturnedAcquisitions(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	buildErr := errors.New("tool environment failed")
	var closed atomic.Int32

	assembly := newAssembly(t.Context(), cfg, func(
		context.Context,
		toolEnvironmentDependencies,
	) (toolEnvironment, error) {
		return toolEnvironment{
			closers: []*teardown.Step{teardown.Terminal(func(context.Context) error {
				closed.Add(1)
				return nil
			})},
		}, buildErr
	})
	host, err := BuildAssembly(t.Context(), assembly)
	if !errors.Is(err, buildErr) {
		t.Fatalf("assemble error = %v, want build failure", err)
	}
	if host != nil {
		t.Fatal("successful rollback returned a Host owner")
	}
	if got := closed.Load(); got != 1 {
		t.Fatalf("returned acquisition close calls = %d, want 1", got)
	}
}

func TestAssemblyFailureRollbackContinuesAfterCloseTimeout(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	// Fail after tool acquisition. OpenInstance receives no Host on this path and
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
	assembly := newAssembly(t.Context(), cfg, func(
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
	})
	assembly.lifetime.shutdownWait = testShutdownWait(t, time.Millisecond)

	failedHost, err := BuildAssembly(t.Context(), assembly)
	if failedHost != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("failed Build = (%v, %v), want nil Host and bounded rollback timeout", failedHost, err)
	}
	<-closerStarted
	if err := CloseAssembly(assembly); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded Assembly.Close = %v, want deadline exceeded", err)
	}

	close(releaseCloser)
	select {
	case <-resourceClosed:
	case <-time.After(time.Second):
		t.Fatal("failed Assembly lost its dependent resource owner after rollback timeout")
	}
}

func TestAssemblyDirectToolsDoNotDependOnAgentResolver(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	var toolClosed atomic.Int32

	assembly := newAssembly(t.Context(), cfg, func(
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
	})
	host, err := BuildAssembly(t.Context(), assembly)
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

	db, err := sqlitestore.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	checkpoints := persistence.NewExecutorCheckpointStore(sqlitestore.NewExecutorCheckpointStore(db))
	mcpServers := sqlitestore.NewMCPServerStore(db)
	return Config{
		Provider:             "anthropic",
		Model:                "claude-test",
		ApprovalMode:         approval.ModeSafe,
		UserHome:             t.TempDir(),
		KnowledgeDirectory:   t.TempDir(),
		DefaultWorkspacePath: t.TempDir(),
		ChatResolver:         testChatResolver(client),
		BuildID:              "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		ConversationStore:    sqlitestore.NewMessageStore(db),
		ProviderRegistry:     sqlitestore.NewProviderStore(db),
		MCPRegistry:          mcpServers,
		MCPOAuthSessions:     mcpServers,
		SessionStore:         sqlitestore.NewSessionStore(db),
		InterruptStore:       persistence.NewInterruptStore(sqlitestore.NewInterruptStore(db)),
		TranscriptStore:      sqlitestore.NewTranscriptStore(db),
		FeedbackStore:        sqlitestore.NewFeedbackStore(db),
		RunStore:             sqlitestore.NewRunStore(db),
		ModelInvocationStore: sqlitestore.NewModelInvocationStore(db),
		ToolInvocationStore:  sqlitestore.NewToolInvocationStore(db),
		ChildRunStartStore:   sqlitestore.NewChildRunStartReservationStore(db),
		ExecutorCheckpoints:  checkpoints,
		Transactor: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlitestore.RunInTx(ctx, db, fn)
		},
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
	if err := cfg.SessionStore.Insert(ctx, value); err != nil {
		t.Fatalf("insert Session: %v", err)
	}
	profile := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Question},
	}
	if err := cfg.RunStore.Admit(ctx, run.Draft{
		RunID: runID, SessionID: sessionID, SegmentID: "seg_open",
		Capabilities: profile, ModelSelection: testsupport.DefaultModelSelection(), CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := cfg.RunStore.Suspend(ctx, testsupport.MustRestoreRun(run.Snapshot{SessionID: sessionID, ID: runID, State: run.Waiting,
		Capabilities: profile,
		CreatedAt:    createdAt, MessageMark: run.UnknownMessageMark}),
		"seg_open", runtimeidentity.CommitID{},
	); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := cfg.TranscriptStore.AppendItem(ctx, testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_park", RunID: runID, SessionID: sessionID,
		Kind:     transcript.QuestionItem,
		Question: question, OccurredAt: parkedAt,
	})); err != nil {
		t.Fatalf("put transcript item: %v", err)
	}
	if err := cfg.InterruptStore.Open(ctx, bootstrapPending(
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
	if err := cfg.ExecutorCheckpoints.SaveCheckpoint(ctx, bootstrapCheckpoint(memberID, sessionID)); err != nil {
		t.Fatalf("save executor checkpoint: %v", err)
	}

	assembly := NewAssembly(t.Context(), cfg)
	host, err := BuildAssembly(ctx, assembly)
	if err != nil {
		t.Fatalf("Assembly.Build: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })

	if pending, listErr := cfg.InterruptStore.List(ctx, sessionID); listErr != nil || len(pending) != 0 {
		t.Fatalf("pending after assemble = (%+v, %v), want none", pending, listErr)
	}
	if _, loadCheckpointErr := cfg.ExecutorCheckpoints.LoadCheckpoint(ctx, memberID); !errors.Is(loadCheckpointErr, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("executor checkpoint after assemble = %v, want not found", loadCheckpointErr)
	}
	runs, err := cfg.RunStore.ListRuns(ctx, sessionID)
	failure, failed := run.Failure{}, false
	if len(runs) == 1 {
		failure, failed = runs[0].Failure()
	}
	if err != nil || len(runs) != 1 || !failed || failure.Kind != run.FailureLost {
		t.Fatalf("runs after assemble = (%+v, %v), want run_lost", runs, err)
	}
}
