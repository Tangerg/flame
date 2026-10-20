package terminal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
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
			t.Setenv("FLAME_A2A_AGENTS", "")
			t.Setenv("FLAME_A2A_RPC_ORIGINS", "")
			workspace := t.TempDir()
			owner := runtimebinding.NewOwner(runtimebinding.Config{
				ProductRoot: runtimefixture.TempDirectory(t), DefaultWorkspacePath: workspace,
				UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()}, ClientVersion: "test",
			})
			connection, err := owner.Connection(t.Context(), settings.Runtime{Mode: settings.RuntimeEmbedded})
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
			session, err := connection.CreateSession(t.Context(), conversation.CreateSession{Workspace: workspace})
			if err != nil {
				t.Fatal(err)
			}
			stream, err := connection.StartRun(t.Context(), prompt.StartRun{
				SessionID: session.ID, Message: prompt.Message{Text: "delegate approval probe"},
				Options: prompt.RunOptions{Provider: "deepseek", Model: "deepseek-chat"},
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
			if !ok || root.Status != protocol.RunStatusWaiting || len(snapshot.Interrupts) != 1 {
				t.Fatalf("waiting tree = %+v", snapshot)
			}
			childID := conversation.InterruptRunID(snapshot.Interrupts[0])
			if childID == root.ID {
				t.Fatal("fixture did not delegate the approval")
			}
			profile := connection.Profile()
			stateDirectory := t.TempDir()
			var stagedCommand replay.CommandID
			if pendingAtStartup {
				store, err := openTestWorkbench(stateDirectory)
				if err != nil {
					t.Fatal(err)
				}
				stagedCommand = "cli_33333333333333333333333333333333"
				policy, policyErr := mutation.PolicyFromProfile(&profile, time.Now)
				if policyErr != nil {
					t.Fatal(policyErr)
				}
				stagedReplay, guardErr := policy.NewGuard()
				if guardErr != nil {
					t.Fatal(guardErr)
				}
				pending := workbench.PendingResume{
					Command: conversation.ResumeRun{CommandID: stagedCommand, RunID: root.ID, Answers: []conversation.InterruptAnswer{{
						ItemID: conversation.InterruptItemID(snapshot.Interrupts[0]), Answer: conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove},
					}}},
					Interrupts: snapshot.Interrupts, Replay: stagedReplay,
				}
				if err := store.StagePendingResume(session.ID, pending, nil); err != nil {
					t.Fatal(err)
				}
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
			}
			openWorkbench, currentWorkbench := observeTestWorkbench(persistentTestWorkbench(stateDirectory))
			backend := &persistedTreeResumeRuntime{
				Connection: connection, workbench: currentWorkbench, sessionID: session.ID,
				observed: make(chan workbench.PendingResume, 1),
			}
			host, stop, _ := runUIFromConfig(t, Config{
				Runtime: backend, RuntimeProfile: &profile, Workspace: workspace,
				SessionID: session.ID, OpenWorkbench: openWorkbench,
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
				return slices.ContainsFunc(finished.Runs, func(run conversation.Run) bool {
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
			if pending.Command.RunID != root.ID || conversation.InterruptRunID(pending.Interrupts[0]) != childID ||
				(pendingAtStartup && pending.Command.CommandID != stagedCommand) {
				t.Fatalf("dispatched root/member review = %+v", pending)
			}
			finished, err := connection.GetSession(t.Context(), session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(finished.Runs) != 2 || !slices.ContainsFunc(finished.Runs, func(run conversation.Run) bool {
				return run.ID == root.ID && run.Status == protocol.RunStatusFinished && run.Outcome.Status == protocol.OutcomeCompleted
			}) {
				t.Fatalf("resumed tree = %+v", finished.Runs)
			}
			store, err := openTestWorkbench(stateDirectory)
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
	workbench func() *workbench.Store
	sessionID string
	observed  chan workbench.PendingResume
}

func (r *persistedTreeResumeRuntime) ResumeRun(ctx context.Context, command conversation.ResumeRun) (conversation.SegmentStream, error) {
	store := r.workbench()
	pending, found := store.PendingResume(r.sessionID)
	if found && pending.Command.Equal(command) {
		r.observed <- pending
	}
	return r.Connection.ResumeRun(ctx, command)
}
