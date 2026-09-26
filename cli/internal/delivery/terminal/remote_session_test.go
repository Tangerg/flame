package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/workbenchstate"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/application/extensions"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/input"
)

func TestRemoteSessionAttachmentsStayLocalAcrossSwitchAndRelocation(t *testing.T) {
	local := t.TempDir()
	path := filepath.Join(local, "context.txt")
	if err := os.WriteFile(path, []byte("client context"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	backend := &recordingRuntime{Runtime: runtimefixture.New()}
	backend.Instant = true
	backend.Script = stableCompletedScript
	first, err := backend.CreateSession(t.Context(), agent.CreateSession{
		Title: "Remote first", Workspace: `C:\server\first`,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.CreateSession(t.Context(), agent.CreateSession{
		Title: "Remote second", Workspace: "/server/second",
	})
	if err != nil {
		t.Fatal(err)
	}
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, SessionID: first.ID, LocalDirectory: local, DetachOnExit: true,
	})
	host.Shows(t, "Ask flame")
	attachAndRun := func(sessionID, text string) {
		t.Helper()
		host.Type("/attach context.txt")
		host.Press(input.Enter)
		host.Shows(t, "attached context.txt")
		host.Type(text)
		host.Press(input.Enter)
		awaitState(t, "remote run attachment", func() bool {
			return backend.startInput().Message.Text == text
		})
		started := backend.startInput()
		if started.SessionID != sessionID || len(started.Message.Attachments) != 1 ||
			started.Message.Attachments[0].Path != wantPath {
			t.Fatalf("remote run input = %+v, want local attachment %q in %s", started, wantPath, sessionID)
		}
		awaitState(t, "remote run completion", func() bool {
			snapshot, readErr := backend.GetSession(t.Context(), sessionID)
			return readErr == nil && snapshot.Session.Status == protocol.SessionStatusIdle
		})
		host.Shows(t, "complete")
	}
	attachAndRun(first.ID, "first local attachment")
	host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl})
	host.Shows(t, "Sessions")
	host.Type("Remote second")
	host.Shows(t, "Remote second")
	host.Press(input.Enter)
	host.Shows(t, "session · Remote second")
	attachAndRun(second.ID, "second local attachment")
	const relocated = `D:\server\relocated`
	host.Type("/relocate " + relocated)
	host.Press(input.Enter)
	awaitState(t, "remote session relocation", func() bool {
		snapshot, readErr := backend.GetSession(t.Context(), second.ID)
		return readErr == nil && snapshot.Session.Workspace.Path == relocated
	})
	host.Shows(t, relocated)
	attachAndRun(second.ID, "relocated local attachment")
	stop()
}

func TestRemoteSessionEditorAndDocumentsUseTheLocalDirectory(t *testing.T) {
	local := t.TempDir()
	wantDirectory, err := filepath.EvalSymlinks(local)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLAME_EDITOR", `sh -c 'printf "local authoring %s" "$(pwd -P)" > "$0"'`)
	backend := &recordingRuntime{Runtime: runtimefixture.New()}
	backend.Instant = true
	backend.Script = stableCompletedScript
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, Workspace: `Z:\server\project`, LocalDirectory: local,
		DetachOnExit: true, Transfers: outputTransferStub{},
	})
	host.Shows(t, "Ask flame")
	host.Type("/editor")
	host.Press(input.Enter)
	host.Shows(t, "updated prompt from external editor")
	host.Press(input.Enter)
	awaitState(t, "locally edited prompt", func() bool {
		return backend.startCount() == 1
	})
	if got := backend.startInput().Message.Text; got != "local authoring "+wantDirectory {
		t.Fatalf("editor working directory = %q, want %q", got, wantDirectory)
	}
	host.Shows(t, "complete")
	host.Type("/export json portable.json")
	host.Press(input.Enter)
	host.Shows(t, "exported session")
	if _, err := os.Stat(filepath.Join(local, "portable.json")); err != nil {
		t.Fatal(err)
	}
	host.Type("/import portable.json")
	host.Press(input.Enter)
	host.Shows(t, "Import session")
	host.Press(input.Esc)
	stop()
}

func TestRemotePluginCommandsReceiveLocalAndRuntimeDirectories(t *testing.T) {
	local := t.TempDir()
	const remote = `C:\server\project`
	backend := runtimefixture.New()
	created, err := backend.CreateSession(t.Context(), agent.CreateSession{Workspace: remote})
	if err != nil {
		t.Fatal(err)
	}
	delivered := make(chan CommandRequest, 1)
	plugin := extensions.Plugin{
		ID: "test.locations", Version: "1.0.0", APIVersion: extensions.HostAPIVersion,
		Capabilities: []extensions.Capability{SlashCommands.Capability()},
		Setup: func(scope *extensions.Scope) error {
			_, err := scope.Contribute(SlashCommands, SlashCommand{
				Descriptor: CommandDescriptor{Name: "locations", Title: "inspect command directories"},
				Available: func(request CommandRequest) CommandAvailability {
					if request.Workspace != remote || request.LocalDirectory != local || request.SessionID != created.ID {
						return CommandUnavailable("local and Runtime directories were not supplied")
					}
					return CommandAvailable()
				},
				Execute: func(_ context.Context, request CommandRequest) (CommandResult, error) {
					delivered <- request
					return CommandResult{Message: "plugin directory context received"}, nil
				},
			}, extensions.Contribution{})
			return err
		},
	}
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, SessionID: created.ID, LocalDirectory: local,
		DetachOnExit: true, Plugins: []extensions.Plugin{plugin},
	})
	host.Shows(t, "Ask flame")
	host.Type("/locations")
	host.Press(input.Enter)
	host.Shows(t, "plugin directory context received")
	request := awaitValue(t, delivered, "plugin command directory context")
	if request.Workspace != remote || request.LocalDirectory != local || request.SessionID != created.ID {
		t.Fatalf("plugin execution context = %+v", request)
	}
	stop()
}

func TestSharedTerminalExitLeavesNewAndAttachedRunsAlive(t *testing.T) {
	for _, attached := range []bool{false, true} {
		name := "new run"
		if attached {
			name = "attached run"
		}
		t.Run(name, func(t *testing.T) {
			backend := newSharedTerminalRuntime()
			created, err := backend.CreateSession(t.Context(), agent.CreateSession{Workspace: `C:\server\project`})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cancelFixtureSession(t, backend, created.ID) })
			if attached {
				if _, err := backend.StartRun(t.Context(), testStartRun(created.ID, "another client started this")); err != nil {
					t.Fatal(err)
				}
			}
			host, stop := runUIFromConfig(t, Config{
				Runtime: backend, SessionID: created.ID, LocalDirectory: t.TempDir(), DetachOnExit: true,
			})
			if !attached {
				host.Shows(t, "Ask flame")
				host.Type("this client started it")
				host.Press(input.Enter)
			}
			if attached {
				host.Shows(t, "reconnected")
			} else {
				host.Shows(t, "working")
			}
			stop()
			snapshot, err := backend.GetSession(t.Context(), created.ID)
			if err != nil {
				t.Fatal(err)
			}
			active, ok := snapshot.ActiveRun()
			if !ok || active.Status != protocol.RunStatusRunning {
				t.Fatalf("detached terminal stopped shared execution: %+v", snapshot.Runs)
			}
		})
	}
}

type heldStartReceiptsRuntime struct {
	*idempotentStartRuntime
	hold      int32
	calls     atomic.Int32
	forwarded chan agent.StartRun
}

func (r *heldStartReceiptsRuntime) StartRun(ctx context.Context, command agent.StartRun) (agent.SegmentStream, error) {
	opened, err := r.idempotentStartRuntime.StartRun(ctx, command)
	if err != nil || r.calls.Add(1) > r.hold {
		return opened, err
	}
	r.forwarded <- command.Clone()
	<-ctx.Done()
	return agent.SegmentStream{}, context.Cause(ctx)
}

func TestSharedTerminalExitPreservesUnknownStartsAndHonorsExplicitCancellation(t *testing.T) {
	for _, cancelBeforeExit := range []bool{false, true} {
		name := "detach unknown start"
		hold := int32(1)
		if cancelBeforeExit {
			name, hold = "finish explicit cancellation", 2
		}
		t.Run(name, func(t *testing.T) {
			base := newSharedTerminalRuntime()
			created, err := base.CreateSession(t.Context(), agent.CreateSession{Workspace: `C:\server\project`})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cancelFixtureSession(t, base, created.ID) })
			backend := &heldStartReceiptsRuntime{
				idempotentStartRuntime: &idempotentStartRuntime{Runtime: base},
				hold:                   hold, forwarded: make(chan agent.StartRun, 2),
			}
			local, state := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(local, "context.txt"), []byte("local input"), 0o600); err != nil {
				t.Fatal(err)
			}
			profile := steerReplayTestProfile(t, created.Workspace.Path)
			host, stop := runUIFromConfig(t, Config{
				Runtime: backend, RuntimeProfile: &profile, SessionID: created.ID,
				LocalDirectory: local, StateDirectory: state, DetachOnExit: true,
			})
			host.Shows(t, "Ask flame")
			host.Type("/attach context.txt")
			host.Press(input.Enter)
			host.Shows(t, "attached context.txt")
			host.Type("preserve this exact input")
			host.Press(input.Enter)
			original := awaitValue(t, backend.forwarded, "unacknowledged accepted start")
			if len(original.Input) == 0 {
				t.Fatal("attachment input was not frozen before dispatch")
			}
			if cancelBeforeExit {
				host.Press(input.Esc)
				awaitValue(t, backend.forwarded, "explicit cancellation reconciliation")
			}
			stop()
			store, err := workbenchstate.Open(state)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			pending := store.PendingRuns(created.ID)
			snapshot, err := base.GetSession(t.Context(), created.ID)
			if err != nil {
				t.Fatal(err)
			}
			attempts := backend.attempts()
			for _, attempt := range attempts {
				if !attempt.Equal(original) {
					t.Fatalf("start changed while exiting: %+v, original %+v", attempt, original)
				}
			}
			if cancelBeforeExit {
				if len(attempts) != 3 || len(pending) != 0 || len(snapshot.Runs) != 1 ||
					snapshot.Runs[0].Outcome.Status != protocol.OutcomeCanceled {
					t.Fatalf("explicit cancellation was not settled: attempts %d, pending %+v, runs %+v", len(attempts), pending, snapshot.Runs)
				}
				return
			}
			if len(attempts) != 1 || len(pending) != 1 || pending[0].State != workbench.PendingRunDispatching ||
				pending[0].CancelCommandID != "" || !pending[0].Command.Equal(original) {
				t.Fatalf("detach changed unknown delivery: attempts %d, pending %+v", len(attempts), pending)
			}
			if active, ok := snapshot.ActiveRun(); !ok || active.Status != protocol.RunStatusRunning {
				t.Fatalf("detach canceled unacknowledged execution: %+v", snapshot.Runs)
			}
		})
	}
}

type openingCancellationReplayRuntime struct {
	*idempotentStartRuntime
	mu           sync.Mutex
	cancellation agent.CancelRun
	receipt      agent.RunCancellation
}

func (r *openingCancellationReplayRuntime) CancelRun(ctx context.Context, command agent.CancelRun) (agent.RunCancellation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancellation.CommandID != "" {
		if command != r.cancellation {
			return agent.RunCancellation{}, agent.ErrCommandConflict
		}
		return r.receipt, nil
	}
	receipt, err := r.Runtime.CancelRun(ctx, command)
	if err != nil {
		return agent.RunCancellation{}, err
	}
	r.cancellation, r.receipt = command, receipt
	return agent.RunCancellation{}, agent.ErrCommandOutcomeUnknown
}

func TestOpeningCancellationReplaysTheSamePayloadAfterCloseAndRestart(t *testing.T) {
	base := newSharedTerminalRuntime()
	backend := &openingCancellationReplayRuntime{idempotentStartRuntime: &idempotentStartRuntime{Runtime: base}}
	const sessionID = "ses_demo_1"
	t.Cleanup(func() { cancelFixtureSession(t, base, sessionID) })
	state := t.TempDir()
	store, err := workbenchstate.Open(state)
	if err != nil {
		t.Fatal(err)
	}
	command := testStartRun(sessionID, "cancel an unconfirmed opening")
	command.CommandID = "cli_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	stageDispatchingRun(t, store, command)
	if _, err := store.MarkPendingRunCanceling(sessionID, command.CommandID, durableCommandReplayGuard(t)); err != nil {
		t.Fatal(err)
	}
	pending := store.PendingRuns(sessionID)[0]
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	profile := steerReplayTestProfile(t, "/tmp/flame-cli-test")
	closing := &app{runtime: backend, runtimeProfile: &profile}
	if err := closing.cancelOpeningRunNow(t.Context(), pending); !errors.Is(err, agent.ErrCommandOutcomeUnknown) {
		t.Fatalf("terminal-close cancellation = %v, want unknown acknowledgement", err)
	}
	host, stop := runUIFromConfig(t, Config{
		Runtime: backend, RuntimeProfile: &profile, SessionID: sessionID,
		LocalDirectory: t.TempDir(), StateDirectory: state, DetachOnExit: true,
	})
	host.Shows(t, "canceled")
	awaitState(t, "the exact cancellation to be confirmed after restart", func() bool {
		recovered, openErr := workbenchstate.Open(state)
		if openErr != nil {
			return false
		}
		defer recovered.Close()
		return len(recovered.PendingRuns(sessionID)) == 0
	})
	stop()
}

func newSharedTerminalRuntime() *runtimefixture.Runtime {
	backend := runtimefixture.New()
	backend.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{
			Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: protocol.OutcomeCompleted}},
		}}}
	}
	return backend
}

func cancelFixtureSession(t *testing.T, backend *runtimefixture.Runtime, sessionID string) {
	t.Helper()
	snapshot, err := backend.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Error(err)
		return
	}
	if active, ok := snapshot.ActiveRun(); ok {
		if _, err := backend.CancelRun(context.Background(), agent.CancelRun{RunID: active.ID}); err != nil {
			t.Error(err)
		}
	}
}

func TestRemoteWorkspaceInputsAndPickerPreserveRuntimePaths(t *testing.T) {
	application := &app{localDirectory: t.TempDir(), session: sessionState{current: agent.Session{
		Workspace: workspace.Workspace{Path: `C:\remote\current`},
	}}}
	for _, path := range []string{`D:\remote\new`, `\\server\share\project`, "/remote/unavailable", "~/remote", "../project"} {
		got, err := application.resolveWorkspaceInput(path)
		if err != nil || got != path {
			t.Fatalf("remote workspace input %q = %q, %v", path, got, err)
		}
	}
	if _, err := application.resolveWorkspaceInput(" "); err == nil {
		t.Fatal("empty remote workspace input was accepted")
	}
	first := `C:\remote\first`
	current := `C:\remote\current`
	choices := mergeWorkspaceChoices([]workspace.Summary{
		{Workspace: workspace.Workspace{Path: first, Availability: protocol.WorkspaceAvailable}},
		{Workspace: workspace.Workspace{Path: current, Availability: protocol.WorkspaceAvailable}},
	}, nil, current)
	if len(choices) != 2 || choices[0].workspace.Path != current || !choices[0].current || choices[1].current {
		t.Fatalf("remote workspace identities = %+v", choices)
	}
}

func TestScheduleFormsPreserveForeignRuntimeWorkspaceReferences(t *testing.T) {
	const path = `D:\server\scheduled`
	draft := scheduleFormDraft{instructions: "review", workspace: path, cron: defaultScheduleCron}
	request, err := draft.candidate()
	if err != nil || request.Workspace == nil || request.Workspace.Path != path {
		t.Fatalf("remote schedule candidate = %+v, %v", request, err)
	}
	original := protocol.Schedule{ID: "sch_review", Revision: 1, Instructions: "review", Cron: defaultScheduleCron}
	requestPatch, changed, err := draft.patch(original)
	if err != nil || !changed || requestPatch.Workspace == nil || requestPatch.Workspace.Path != path {
		t.Fatalf("remote schedule update = %+v, changed %t, %v", requestPatch, changed, err)
	}
}
