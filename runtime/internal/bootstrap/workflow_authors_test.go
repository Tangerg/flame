package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/goals"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	"github.com/Tangerg/flame/packages/go-sdk/planworkflow"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/runoutcome"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func activateWorkflow(r *poMCPRuntime, source, key, role, id string) *protocol.PluginInstallation {
	r.t.Helper()
	p := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, key+"-install").(*protocol.PluginInstallation)
	p = r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: p.ID, Enabled: true}, key+"-enable").(*protocol.PluginInstallation)
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: role, InstallationID: p.ID, Contribution: id}, key+"-select")
	return p
}

func installWorkflowAuthors(t *testing.T, r *poMCPRuntime, name, role, id string) (*protocol.PluginInstallation, func() *protocol.PluginInstallation) {
	t.Helper()
	install := func(source, key string) *protocol.PluginInstallation {
		return activateWorkflow(r, source, key, role, id)
	}
	source := filepath.Join(firstPartyPackages, name)
	first := install(source, name+"-a")
	return first, func() *protocol.PluginInstallation {
		second := filepath.Join(t.TempDir(), name)
		if err := os.CopyFS(second, os.DirFS(source)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(second, "edition.txt"), []byte("second authored edition"), 0600); err != nil {
			t.Fatal(err)
		}
		authorPluginPackage(t, second)
		return install(second, name+"-b")
	}
}

func TestExistingWorkflowsRetainTheirOriginalAuthors(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	calls := r.host.application.delivery.PluginCalls.(*pluginservices.CommandCalls)
	ctx := protocolLifecycleContext(t.Context())
	var unbound extension.ContributionSource
	t.Run("plan", func(t *testing.T) {
		first, chooseSecond := installWorkflowAuthors(t, r, "plans", planworkflow.Reference, "plan-workflow")
		proposal := planworkflow.Proposal{Steps: []planworkflow.Step{{Description: "check originally proposed steps", Status: planworkflow.PlanStatusPending}}}
		issued, err := calls.AuthorPlan(ctx, unbound, proposal)
		if err != nil || issued.Source.InstallationID != first.ID {
			t.Fatalf("first author=%v, %v", issued.Source, err)
		}
		second := chooseSecond()
		if _, err := calls.ReviewPlan(ctx, issued.Source, issued.Document); err != nil {
			t.Fatalf("selection change withdrew the still-admitted original: %v", err)
		}
		r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: first.ID, Enabled: false}, "withdraw-original-plan")
		if _, err := calls.ReviewPlan(ctx, issued.Source, issued.Document); !errors.Is(err, plugin.ErrUnavailable) {
			t.Fatalf("replacement reviewed original: %v", err)
		}
		newDocument, err := calls.AuthorPlan(ctx, unbound, proposal)
		if err != nil || newDocument.Source.InstallationID != second.ID {
			t.Fatalf("new plan attributed to %v, %v", newDocument.Source, err)
		}
	})
	t.Run("goal", func(t *testing.T) {
		first, chooseSecond := installWorkflowAuthors(t, r, "goals", goalworkflow.Reference, "goal-workflow")
		create := goalworkflow.Request{Signal: goalworkflow.Start, Objective: "capture the original owner"}
		issued, err := calls.DecideGoal(ctx, unbound, create)
		if err != nil || issued.Next == nil || issued.Source.InstallationID != first.ID {
			t.Fatalf("first author=%v, %v", issued.Source, err)
		}
		next := goalworkflow.Request{Signal: goalworkflow.NextRun, Current: issued.Next, Ordinal: 1}
		second := chooseSecond()
		round, err := calls.DecideGoal(ctx, issued.Source, next)
		if err != nil || round.Source != issued.Source || round.Instructions == "" {
			t.Fatalf("existing goal used %v, %v", round.Source, err)
		}
		r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: first.ID, Enabled: false}, "withdraw-original-goal")
		if _, err := calls.DecideGoal(ctx, issued.Source, next); !errors.Is(err, plugin.ErrUnavailable) {
			t.Fatalf("replacement advanced original goal: %v", err)
		}
		session, err := r.api.CreateSession(ctx, protocol.CreateSessionRequest{Workspace: &protocol.WorkspaceRef{Path: t.TempDir()}})
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		original, err := (goal.Snapshot{SessionID: session.ID, CreatedAt: now}).Publish(*issued.Next, issued.Source, "original-goal", now)
		if err != nil {
			t.Fatal(err)
		}
		initial, err := goal.Unwritten(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		opening, err := goal.NewReplacement(initial.Version(), original)
		if err != nil {
			t.Fatal(err)
		}
		store := cfg.stores.Goals
		if ok, err := store.Save(ctx, opening); err != nil || !ok {
			t.Fatalf("original goal commit: %t %v", ok, err)
		}
		recorder, err := goals.NewRunRecorder(store)
		if err != nil {
			t.Fatal(err)
		}
		finish := now.Add(time.Second)
		raw := testsupport.MustRestoreRun(run.Snapshot{SessionID: session.ID, ID: "run_original_receipt", GoalIncarnationID: original.IncarnationID(), Ending: testsupport.EndingFor(run.OutcomeFailed), CreatedAt: now, FinishedAt: finish, UpdatedAt: finish, MessageMark: run.MessageMarkAt(0)})
		if err := cfg.stores.Transactor(ctx, func(ctx context.Context) error {
			if err := cfg.stores.Runs.Restore(ctx, raw); err != nil {
				return err
			}
			return recorder.RecordRun(ctx, raw)
		}); err != nil {
			t.Fatal(err)
		}
		if err := recorder.Drain(ctx, store, calls, session.ID, nil); !errors.Is(err, plugin.ErrUnavailable) {
			t.Fatalf("substitute consumed a pending receipt: %v", err)
		}
		if p, found, err := store.PendingGoalRun(ctx, session.ID); err != nil || !found || !p.Owns(original) {
			t.Fatalf("lost original receipt: %+v %t %v", p, found, err)
		}
		created, err := calls.DecideGoal(ctx, unbound, create)
		if err != nil || created.Source.InstallationID != second.ID {
			t.Fatalf("new goal attributed to %v, %v", created.Source, err)
		}
		r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: first.ID, Enabled: true}, "restore-original-goal")
		if err := recorder.Drain(ctx, store, calls, session.ID, nil); err != nil {
			t.Fatal(err)
		}
		head, err := store.Get(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		paused, found := head.Goal()
		if !found || paused.IncarnationID() != original.IncarnationID() || paused.Revision() != 2 || paused.Status() != goal.StatusPaused || paused.Reason().Code() != goal.ReasonRunNotCompleted || paused.Used().Runs != 1 {
			t.Fatalf("the original author did not consume its own terminal evidence: %+v", paused.Snapshot())
		}
		if err := recorder.RecordRun(ctx, raw); err != nil {
			t.Fatal(err)
		}
		if _, pending, err := store.PendingGoalRun(ctx, session.ID); err != nil || pending {
			t.Fatalf("the same receipt was payable twice: %t %v", pending, err)
		}
		terminal := goalworkflow.Request{Signal: goalworkflow.TerminalRun, Current: issued.Next, Outcome: runoutcome.Failed}
		settled, err := calls.DecideGoal(ctx, issued.Source, terminal)
		if err != nil || settled.Source != issued.Source || settled.Next == nil || settled.Next.Reason != goalworkflow.GoalReasonRunNotCompleted {
			t.Fatalf("original author did not resume its receipt: %+v %v", settled, err)
		}

	})
}
