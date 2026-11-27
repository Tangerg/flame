package bootstrap

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestGoalContinuesAfterEarlierManualQuestionIsCanceled(t *testing.T) {
	t.Setenv("FLAME_HOME", testsupport.TempDirectory(t))
	model := newLifecycleModel()
	host, api := openProtocolRuntime(t, model)
	poMCPActivate(&poMCPRuntime{t: t, host: host, api: api, endpoint: host.Endpoint()}, filepath.Join(firstPartyPackages, "goals"))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: t.TempDir()}, Title: "manual question before Goal",
	})
	if err != nil {
		t.Fatal(err)
	}
	manual, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Ask before continuing."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForSignal(t, model.firstCallStarted, "manual model call")
	close(model.releaseFirstCall)
	waitForRunEvents(t, collectRunEvents(events), "manual question boundary")
	waiting, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: manual.RunID})
	if err != nil || waiting.Status != protocol.RunStatusWaiting {
		t.Fatalf("manual Run = %+v, %v; want waiting", waiting, err)
	}
	started, err := api.StartGoal(ctx, protocol.StartGoalRequest{
		SessionID: session.ID, Objective: "Continue after the earlier manual question is settled.",
		Provider: "anthropic", Model: "claude-test",
	})
	if err != nil || started.Status != protocol.GoalActive {
		t.Fatalf("Goal start = %+v, %v", started, err)
	}
	if _, err := api.CancelRun(ctx, protocol.CancelRunRequest{RunID: manual.RunID, Reason: "continue with the Goal"}); err != nil {
		t.Fatal(err)
	}
	waitForSignal(t, model.resumedCallStarted, "first Goal-owned model call")
	page, err := api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID})
	if err != nil || len(page.Data) != 2 {
		t.Fatalf("Runs after manual cancellation = %+v, %v; want the manual and Goal Runs", page, err)
	}
	request := protocol.GetGoalRequest{SessionID: session.ID}
	if err := api.StopGoal(ctx, started.GoalTarget); err != nil {
		t.Fatal(err)
	}
	stopped, err := api.GetGoal(ctx, request)
	if err != nil || stopped == nil || stopped.Status != protocol.GoalPaused || stopped.Used.Runs != 1 {
		t.Fatalf("Goal after Stop = %+v, %v; want paused with only the Goal-owned Run charged", stopped, err)
	}
}

func TestGoalWaitsWhenAManualRunWinsAdmission(t *testing.T) {
	model := newLifecycleModel()
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	catalog := &goalAdmissionCatalog{
		modelProviderAdapters: testChatResolver(client),
		entered:               make(chan struct{}),
		proceed:               make(chan struct{}),
	}
	cfg := poMCPConfig(t)
	cfg.modelProviders = catalog
	runtime := poMCPOpen(t, cfg)
	poMCPActivate(runtime, filepath.Join(firstPartyPackages, "goals"))
	allowGoal := sync.OnceFunc(func() { close(catalog.proceed) })
	defer allowGoal()
	ctx, stop := context.WithTimeout(protocolLifecycleContext(t.Context()), lifecycleWaitBudget(t))
	defer stop()
	session, err := runtime.api.CreateSession(ctx, protocol.CreateSessionRequest{Title: "Goal competes with manual Run"})
	if err != nil {
		t.Fatal(err)
	}
	started, err := runtime.api.StartGoal(context.WithValue(ctx, goalAdmissionKey{}, true), protocol.StartGoalRequest{
		SessionID: session.ID, Objective: "Continue after the competing manual Run.",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForSignal(t, catalog.entered, "Goal preparation after observing availability")

	manual, events, err := runtime.api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Ask before continuing."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForSignal(t, model.firstCallStarted, "competing manual model call")
	close(model.releaseFirstCall)
	waitForRunEvents(t, collectRunEvents(events), "competing manual question boundary")
	parked, err := runtime.api.GetRun(ctx, protocol.GetRunRequest{RunID: manual.RunID})
	if err != nil || parked.Status != protocol.RunStatusWaiting {
		t.Fatalf("manual Run = %+v, %v, want waiting", parked, err)
	}

	// The parked Run holds the durable slot. The Goal's attempted admission
	// releases its in-memory reservation after detecting that conflict.
	changed := runtime.host.application.admissions.Changes()
	allowGoal()
	waitForSignal(t, changed, "Goal to release its conflicting admission")
	if _, err := runtime.api.CancelRun(ctx, protocol.CancelRunRequest{RunID: manual.RunID}); err != nil {
		t.Fatal(err)
	}
	awaitGoalOwnedModel(t, ctx, runtime, session.ID, model.resumedCallStarted)
	if err := runtime.api.StopGoal(ctx, started.GoalTarget); err != nil {
		t.Fatal(err)
	}
}

func awaitGoalOwnedModel(t *testing.T, ctx context.Context, runtime *poMCPRuntime, sessionID string, called <-chan struct{}) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-called:
			return
		case <-ctx.Done():
			t.Fatalf("Goal did not claim the released admission: %v", ctx.Err())
		case <-ticker.C:
			current, err := runtime.api.GetGoal(ctx, protocol.GetGoalRequest{SessionID: sessionID})
			if err != nil || current == nil || current.Status != protocol.GoalActive {
				t.Fatalf("Goal = %+v, %v, want an active Goal waiting for admission", current, err)
			}
		}
	}
}

type goalAdmissionKey struct{}

type goalAdmissionCatalog struct {
	modelProviderAdapters
	entered, proceed chan struct{}
	paused           atomic.Bool
}

func (c *goalAdmissionCatalog) Profile(ctx context.Context, id string) (modelprovider.Profile, error) {
	if ctx.Value(goalAdmissionKey{}) != nil && c.paused.CompareAndSwap(false, true) {
		close(c.entered)
		select {
		case <-c.proceed:
		case <-ctx.Done():
			return modelprovider.Profile{}, ctx.Err()
		}
	}
	return c.modelProviderAdapters.Profile(ctx, id)
}
