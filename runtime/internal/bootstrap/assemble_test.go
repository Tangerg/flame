package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
	"github.com/Tangerg/flame/qimu/runner"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/pluginhost"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestNewRequiresRuntimeDependencies(t *testing.T) {
	tests := []struct {
		name string
		edit func(*assemblyConfig)
		want string
	}{
		{
			name: "stores",
			edit: func(cfg *assemblyConfig) { cfg.stores = nil },
			want: "runtime: Stores is required",
		},
		{
			name: "user home",
			edit: func(cfg *assemblyConfig) {
				cfg.userHome = ""
			},
			want: "runtime: UserHome is required",
		},
		{
			name: "relative user home",
			edit: func(cfg *assemblyConfig) {
				cfg.userHome = "relative-home"
			},
			want: "runtime: UserHome must be absolute",
		},
		{
			name: "default workspace path",
			edit: func(cfg *assemblyConfig) {
				cfg.defaultWorkspacePath = ""
			},
			want: "runtime: DefaultWorkspacePath is required",
		},
		{
			name: "relative default workspace path",
			edit: func(cfg *assemblyConfig) {
				cfg.defaultWorkspacePath = "relative-workspace"
			},
			want: "runtime: DefaultWorkspacePath must be absolute",
		},
		{
			name: "missing skills user directory",
			edit: func(cfg *assemblyConfig) { cfg.skillsUserDir = "" },
			want: "runtime: SkillsUserDir is required",
		},
		{
			name: "relative skills user directory",
			edit: func(cfg *assemblyConfig) {
				cfg.skillsUserDir = "relative-skills"
			},
			want: "runtime: SkillsUserDir must be absolute",
		},
		{
			name: "relative sandbox directory",
			edit: func(cfg *assemblyConfig) {
				cfg.sandboxDir = "relative-sandbox"
			},
			want: "runtime: SandboxDir must be absolute when set",
		},
		{
			name: "relative sandbox read-only path",
			edit: func(cfg *assemblyConfig) {
				cfg.sandboxReadOnlyPaths = []string{"relative-read-only"}
			},
			want: "runtime: SandboxReadOnlyPaths[0] must be absolute when set",
		},
		{
			name: "relative checkpoint directory",
			edit: func(cfg *assemblyConfig) {
				cfg.checkpointDir = "relative-checkpoints"
			},
			want: "runtime: CheckpointDir must be absolute when set",
		},
		{
			name: "chat resolver",
			edit: func(cfg *assemblyConfig) {
				cfg.chatResolver = nil
			},
			want: "runtime: ChatResolver is required",
		},
		{
			name: "typed nil chat resolver",
			edit: func(cfg *assemblyConfig) { cfg.chatResolver = (*modeladapter.ChatResolver)(nil) },
			want: "runtime: ChatResolver is required",
		},
		{
			name: "provider registry",
			edit: func(cfg *assemblyConfig) {
				cfg.providerRegistry = nil
			},
			want: "runtime: ProviderRegistry is required",
		},
		{
			name: "typed nil provider registry",
			edit: func(cfg *assemblyConfig) { cfg.providerRegistry = (*persistence.ProviderStore)(nil) },
			want: "runtime: ProviderRegistry is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := runtimeConfigWithRequiredDeps(t)
			tt.edit(&cfg)

			_, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildToolEnvironment)
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
	cfg.resources = []terminalResource{closerFunc(func() error {
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
		toolRuntime.closers = append(toolRuntime.closers, terminalStep(func(context.Context) error {
			toolClosed.Add(1)
			return nil
		}))
		return toolRuntime, buildErr
	}
	host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildTools)
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

func TestAssemblyRejectsInvalidConstructionBeforeStartupReconciliation(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*assemblyConfig)
	}{
		{name: "default model", edit: func(cfg *assemblyConfig) { cfg.provider = "" }},
		{name: "typed nil chat resolver", edit: func(cfg *assemblyConfig) { cfg.chatResolver = (*modeladapter.ChatResolver)(nil) }},
		{name: "typed nil provider registry", edit: func(cfg *assemblyConfig) { cfg.providerRegistry = (*persistence.ProviderStore)(nil) }},
		{name: "session-only approval mode", edit: func(cfg *assemblyConfig) { cfg.approvalMode = approval.ModePlan }},
		{name: "invalid offload policy", edit: func(cfg *assemblyConfig) { cfg.toolResultOffloadEnabled = true }},
		{name: "missing feedback store", edit: func(cfg *assemblyConfig) { cfg.stores.Feedback = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := runtimeConfigWithRequiredDeps(t)
			store := cfg.stores.ToolResults
			id := transcript.ToolResultID("STAGED234")
			if err := store.Stage(t.Context(), transcript.ToolResultStage{
				ID: id, SessionID: "ses_staged", Body: "unbound",
			}); err != nil {
				t.Fatalf("stage tool result: %v", err)
			}
			test.edit(&cfg)
			host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildToolEnvironment)
			if host != nil || err == nil {
				t.Fatalf("assembly = (%v, %v), want invalid construction rejected", host, err)
			}
			if _, found, fetchErr := store.Fetch(t.Context(), "ses_staged", id); fetchErr != nil || !found {
				t.Fatalf("staged tool result after rejected config = (found %v, err %v), want (true, nil)", found, fetchErr)
			}
		})
	}
}

func TestAssemblyReclaimsStagedToolResultsAfterRecoveryAdmission(t *testing.T) {
	cfg := runtimeConfigWithRequiredDeps(t)
	id := transcript.ToolResultID("STAGED234")
	if err := cfg.stores.ToolResults.Stage(t.Context(), transcript.ToolResultStage{
		ID: id, SessionID: "ses_staged", Body: "unbound",
	}); err != nil {
		t.Fatal(err)
	}
	host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildToolEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, found, err := cfg.stores.ToolResults.Fetch(t.Context(), "ses_staged", id); err != nil || !found {
		t.Fatalf("construction reclaimed before recovery admission: found=%v error=%v", found, err)
	}
	if err := host.application.recoverUncertainExecutions(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, found, err := cfg.stores.ToolResults.Fetch(t.Context(), "ses_staged", id); err != nil || found {
		t.Fatalf("staged tool result after admitted recovery = (found %v, err %v), want (false, nil)", found, err)
	}
}

func TestAssemblyRetainsUnreferencedPluginReleaseUntilConstructionSucceeds(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	source := filepath.Join(firstPartyPackages, "memory")
	r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-memory")
	r.close()
	installations, err := cfg.stores.Installations.List(t.Context())
	if err != nil || len(installations) != 1 {
		t.Fatalf("installations = %+v, %v", installations, err)
	}
	installed := installations[0]
	framework := pluginhost.New(t, cfg.stores)
	if err := framework.Host.Uninstall(t.Context(), qimu.InstallationID(installed.Installation.ID().String())); err != nil {
		t.Fatal(err)
	}
	if _, err := framework.Host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	content, err := distribution.Open(distribution.Config{
		Directory: filepath.Join(cfg.stores.DataDirectory, "plugins", "qimu"), Admission: cfg.stores.Admission,
		Trust: func(context.Context, distribution.Source, distribution.Manifest, distribution.Lock) error { return nil },
		Retained: func(ctx context.Context, digest qimu.Digest) (bool, error) {
			return retainedPluginContent(ctx, cfg.stores, digest)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, pathErr := content.Path(qimu.Digest(installed.Selected.Digest().String()))
	if err := errors.Join(pathErr, content.Close(t.Context())); err != nil {
		t.Fatal(err)
	}
	feedback := cfg.stores.Feedback
	cfg.stores.Feedback = nil
	if host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildToolEnvironment); host != nil || err == nil {
		t.Fatalf("assembly = (%v, %v), want missing feedback rejected", host, err)
	}
	if _, err := cfg.stores.PluginReleases.Get(t.Context(), installed.Selected.Digest()); err != nil {
		t.Fatalf("failed construction reclaimed the retained release: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("failed construction reclaimed original content: %v", err)
	}
	cfg.stores.Feedback = feedback
	reopenPluginStores(t, &cfg)
	poMCPOpen(t, cfg)
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("admitted startup retained unreferenced content: %v", err)
	}
	if _, err := cfg.stores.PluginReleases.Get(t.Context(), installed.Selected.Digest()); err != nil {
		t.Fatalf("content collection erased historical declaration metadata: %v", err)
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
			closers: []*teardownStep{terminalStep(func(context.Context) error {
				closed.Add(1)
				return nil
			})},
		}, buildErr
	}
	host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildTools)
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
	cfg.resources = []terminalResource{closerFunc(func() error {
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
		toolRuntime.closers = append(toolRuntime.closers, terminalStep(func(context.Context) error {
			close(closerStarted)
			<-releaseCloser
			return nil
		}))
		return toolRuntime, buildErr
	}
	lifetime := newRuntimeLifetime(t.Context(), cfg.resources)
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

func TestAssemblyRejectsMissingToolResolverAndReclaimsResources(t *testing.T) {
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
		toolRuntime.closers = append(toolRuntime.closers, terminalStep(func(context.Context) error {
			toolClosed.Add(1)
			return nil
		}))
		toolRuntime.tools.Resolver = nil
		return toolRuntime, nil
	}
	host, err := assemble(t.Context(), cfg, newRuntimeLifetime(t.Context(), cfg.resources), buildTools)
	if host != nil || err == nil || !strings.Contains(err.Error(), "tool resolver is required") {
		t.Fatalf("assemble = (%v, %v), want missing tool resolver rejected", host, err)
	}
	if got := toolClosed.Load(); got != 1 {
		t.Fatalf("tool closer calls = %d, want 1", got)
	}
}

func runtimeConfigWithRequiredDeps(t *testing.T) assemblyConfig {
	t.Helper()

	client, err := chatclient.New(newReplyStub("ok"), chatclient.Config{})
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}

	workspace := t.TempDir()
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	return assemblyConfig{
		processRunner: testProcessRunner(t),
		stores:        stores,
		provider:      "anthropic", model: "claude-test",
		approvalMode: approval.ModeSafe,
		userHome:     t.TempDir(), defaultWorkspacePath: workspace,
		skillsUserDir:    t.TempDir(),
		chatResolver:     testChatResolver(client),
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
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?", Kind: transcript.QuestionText}}}
	value := testsupport.MustRestoreSession(session.Snapshot{
		ID: sessionID, Workspace: testsupport.MustWorkspace(t.TempDir()), CreatedAt: createdAt, UpdatedAt: createdAt,
	})
	if err := cfg.stores.Sessions.Insert(ctx, value); err != nil {
		t.Fatalf("insert Session: %v", err)
	}
	profile := run.Capabilities{
		InterruptKinds: []run.InterruptKind{run.InterruptQuestion},
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
	failure, failed := run.Failure{}, false
	if len(runs) == 1 {
		failure, failed = runs[0].Failure()
	}
	if err != nil || len(runs) != 1 || !failed || failure.Kind != run.FailureLost {
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
			t.Error(err)
		}
	})
	return value
}
