package terminal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/workbenchstate"
	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/input"
)

func TestTerminalResumesAColdDelegatedApprovalThroughTheWorkbench(t *testing.T) {
	for _, pendingAtStartup := range []bool{false, true} {
		name := "review child approval"
		if pendingAtStartup {
			name = "recover persisted child decision"
		}
		t.Run(name, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(runtimefixture.ServeDelegatedApproval))
			t.Cleanup(provider.Close)
			t.Setenv("FLAME_PROVIDER", "deepseek")
			t.Setenv("FLAME_MODEL", "deepseek-chat")
			t.Setenv("DEEPSEEK_API_KEY", "integration-key")
			t.Setenv("FLAME_BASEURL", provider.URL)
			t.Setenv("FLAME_MCP_SERVERS", "")
			t.Setenv("FLAME_A2A_AGENTS", "")
			t.Setenv("FLAME_A2A_RPC_ORIGINS", "")
			workspace := t.TempDir()
			owner := runtimebinding.NewOwner(runtimebinding.Config{
				ProductRoot: t.TempDir(), DefaultWorkspacePath: workspace,
				UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()}, ClientVersion: "test",
			})
			connection, err := owner.Connection(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := owner.Close(); err != nil {
					t.Error(err)
				}
			})
			if _, err := connection.SetApprovalMode(t.Context(), protocol.ApprovalModeSafe); err != nil {
				t.Fatal(err)
			}
			session, err := connection.CreateSession(t.Context(), agent.CreateSession{Workspace: workspace})
			if err != nil {
				t.Fatal(err)
			}
			stream, err := connection.StartRun(t.Context(), agent.StartRun{
				SessionID: session.ID, Message: agent.Message{Text: "delegate approval probe"},
				Options: agent.RunOptions{Provider: "deepseek", Model: "deepseek-chat"},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, err := range stream.Events {
				if err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := connection.GetSession(t.Context(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			root, ok := snapshot.ActiveRun()
			if !ok || root.Status != protocol.RunStatusWaiting || len(snapshot.Interactions) != 1 {
				t.Fatalf("waiting tree = %+v", snapshot)
			}
			childID := agent.InteractionRunID(snapshot.Interactions[0])
			if childID == root.ID {
				t.Fatal("fixture did not delegate the approval")
			}
			profile := connection.Profile()
			stateDirectory := t.TempDir()
			var stagedCommand agent.CommandID
			if pendingAtStartup {
				store, err := workbenchstate.Open(stateDirectory)
				if err != nil {
					t.Fatal(err)
				}
				stagedCommand = "cli_33333333333333333333333333333333"
				pending := workbench.PendingResume{
					Command: agent.ResumeRun{CommandID: stagedCommand, RunID: root.ID, Answers: []agent.InterruptAnswer{{
						ItemID: agent.InteractionItemID(snapshot.Interactions[0]), Answer: agent.ApprovalAnswer{Decision: protocol.ApprovalApprove},
					}}},
					Interactions: snapshot.Interactions, Replay: commandReplayGuard(&profile),
				}
				if err := store.StagePendingResume(session.ID, pending, nil); err != nil {
					t.Fatal(err)
				}
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
			}
			backend := &persistedTreeResumeRuntime{
				Connection: connection, stateDirectory: stateDirectory, sessionID: session.ID,
				observed: make(chan workbench.PendingResume, 1),
			}
			host, stop := runUIFromConfig(t, Config{
				Runtime: backend, RuntimeProfile: &profile, Workspace: workspace,
				SessionID: session.ID, StateDirectory: stateDirectory,
			})
			if !pendingAtStartup {
				host.Shows(t, "Tool approval")
				host.Press(input.Enter)
			}
			host.Shows(t, "root complete")
			awaitState(t, "delegated root completion", func() bool {
				finished, err := connection.GetSession(t.Context(), session.ID)
				if err != nil {
					return false
				}
				return slices.ContainsFunc(finished.Runs, func(run agent.Run) bool {
					return run.ID == root.ID && run.Status == protocol.RunStatusFinished && run.Outcome.Status == protocol.OutcomeCompleted
				})
			})
			stop()
			var pending workbench.PendingResume
			select {
			case pending = <-backend.observed:
			default:
				t.Fatal("resume reached Runtime without its durable decision")
			}
			if pending.Command.RunID != root.ID || agent.InteractionRunID(pending.Interactions[0]) != childID ||
				(pendingAtStartup && pending.Command.CommandID != stagedCommand) {
				t.Fatalf("dispatched root/member review = %+v", pending)
			}
			finished, err := connection.GetSession(t.Context(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(finished.Runs) != 2 || !slices.ContainsFunc(finished.Runs, func(run agent.Run) bool {
				return run.ID == root.ID && run.Status == protocol.RunStatusFinished && run.Outcome.Status == protocol.OutcomeCompleted
			}) {
				t.Fatalf("resumed tree = %+v", finished.Runs)
			}
			store, err := workbenchstate.Open(stateDirectory)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if _, found := store.PendingResume(session.ID); found {
				t.Fatal("acknowledged root resume remains pending")
			}
		})
	}
}

type persistedTreeResumeRuntime struct {
	*runtimebinding.Connection
	stateDirectory string
	sessionID      string
	observed       chan workbench.PendingResume
}

func (r *persistedTreeResumeRuntime) ResumeRun(ctx context.Context, command agent.ResumeRun) (agent.SegmentStream, error) {
	store, err := workbenchstate.Open(r.stateDirectory)
	if err != nil {
		return agent.SegmentStream{}, err
	}
	pending, found := store.PendingResume(r.sessionID)
	if err := store.Close(); err != nil {
		return agent.SegmentStream{}, err
	}
	if found && pending.Command.Equal(command) {
		r.observed <- pending
	}
	return r.Connection.ResumeRun(ctx, command)
}
