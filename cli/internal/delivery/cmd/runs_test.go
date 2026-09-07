package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestCatalogListCommandsRejectNonPositiveAndOversizedPageFlags(t *testing.T) {
	t.Parallel()
	for _, arguments := range [][]string{
		{"sessions", "ls", "--limit", "0"},
		{"sessions", "ls", "--limit", "101"},
		{"runs", "ls", "--limit", "0"},
		{"runs", "ls", "--limit", "101"},
	} {
		if _, _, err := executeCommand(t, instantRuntime(), "", arguments...); !errors.Is(err, agent.ErrInvalidPageSize) {
			t.Fatalf("%v error = %v, want ErrInvalidPageSize", arguments, err)
		}
	}
}

type recordingRunCatalog struct {
	Runtime
	queries []agent.RunQuery
}

func (r *recordingRunCatalog) ListRuns(ctx context.Context, query agent.RunQuery) (protocol.Page[protocol.RunRef], error) {
	r.queries = append(r.queries, query)
	return r.Runtime.ListRuns(ctx, query)
}

func TestRunsListConsumesFiltersAndStableJSON(t *testing.T) {
	base := instantRuntime()
	runtime := &recordingRunCatalog{Runtime: base}
	out, errOut, err := executeCommand(t, runtime, "", "runs", "ls",
		"--session", "ses_demo_1", "--status", "finished", "--include-descendants", "--limit", "7", "--json",
	)
	if err != nil {
		t.Fatalf("runs ls: %v", err)
	}
	if errOut != "" {
		t.Fatalf("runs ls stderr = %q", errOut)
	}
	if len(runtime.queries) != 1 {
		t.Fatalf("queries = %+v", runtime.queries)
	}
	query := runtime.queries[0]
	rows, rowsErr := query.PageSize.Rows()
	if query.SessionID != "ses_demo_1" || len(query.Statuses) != 1 || query.Statuses[0] != protocol.RunStatusFinished ||
		!query.IncludeDescendants || rowsErr != nil || rows != 7 {
		t.Fatalf("query = %+v", query)
	}
	var page protocol.Page[protocol.RunRef]
	if err := json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatalf("runs list output: %v\n%s", err, out)
	}
	if len(page.Data) != 1 || page.Data[0].ID != "run_demo_history" || page.Data[0].SessionID != "ses_demo_1" || page.Data[0].Status != "finished" {
		t.Fatalf("page = %+v", page)
	}
	if strings.Contains(out, `"ID"`) {
		t.Fatalf("runs list leaked Go field names:\n%s", out)
	}
}

func TestRunsListKeepsPaginationOutOfMachineOutput(t *testing.T) {
	runtime := instantRuntime()
	runtime.Script = shortCompletedScript
	stream, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: "ses_demo_1", Message: agent.Message{Text: "newer run"},
		Options: agent.RunOptions{Limits: agent.UnlimitedRunLimits()},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, streamErr := range stream.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}

	textOut, textErr, err := executeCommand(t, runtime, "", "runs", "ls", "--session", "ses_demo_1", "--limit", "1")
	if err != nil || len(strings.Split(strings.TrimSpace(textOut), "\n")) != 1 || !strings.Contains(textErr, "more runs: --cursor") {
		t.Fatalf("text page = %q, stderr %q, %v", textOut, textErr, err)
	}
	jsonOut, jsonErr, err := executeCommand(t, runtime, "", "runs", "ls", "--session", "ses_demo_1", "--limit", "1", "--json")
	if err != nil || jsonErr != "" || !strings.Contains(jsonOut, `"nextCursor"`) {
		t.Fatalf("JSON page = %q, stderr %q, %v", jsonOut, jsonErr, err)
	}
}

func TestRunsListRejectsAnInvalidStatusBeforeOpeningTheRuntime(t *testing.T) {
	var opened bool
	provider := runtimeProvider{open: func(context.Context) (Runtime, *runtimebinding.Profile, error) {
		opened = true
		return instantRuntime(), nil, nil
	}}
	command := newRunsListCommand(provider)
	command.SetOut(&strings.Builder{})
	command.SetErr(&strings.Builder{})
	command.SetArgs([]string{"--status", "paused"})
	if err := command.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("invalid status error = %v", err)
	}
	if opened {
		t.Fatal("invalid status opened the runtime")
	}
}

func TestRunsListRejectsDescendantsBeforeCallingAnUnnegotiatedRuntime(t *testing.T) {
	t.Parallel()
	profile := commandRuntimeProfile(t, func(discovery *protocol.DiscoverResponse, client *protocol.ClientCapabilities) {
		discovery.Capabilities.Features[protocol.FeatureSubagents] = protocol.FeatureCapability{
			ClientOptIn: true,
		}
	})
	runtime := &recordingRunCatalog{Runtime: instantRuntime()}
	provider := runtimeProvider{open: func(context.Context) (Runtime, *runtimebinding.Profile, error) {
		return runtime, new(profile), nil
	}}
	command := newRunsListCommand(provider)
	command.SetOut(&strings.Builder{})
	command.SetErr(&strings.Builder{})
	command.SetArgs([]string{"--include-descendants"})
	if err := command.ExecuteContext(t.Context()); err == nil || !strings.Contains(err.Error(), "subagents") {
		t.Fatalf("runs list error = %v", err)
	}
	if len(runtime.queries) != 0 {
		t.Fatalf("unnegotiated descendant query reached runtime: %+v", runtime.queries)
	}
}

func TestRunsShowUsesDirectRunRead(t *testing.T) {
	runtime := instantRuntime()
	out, _, err := executeCommand(t, runtime, "", "runs", "show", "run_demo_history", "--json")
	if err != nil {
		t.Fatalf("runs show: %v", err)
	}
	var run protocol.RunRef
	if err := json.Unmarshal([]byte(out), &run); err != nil {
		t.Fatalf("runs show output: %v\n%s", err, out)
	}
	if run.ID != "run_demo_history" || run.Status != "finished" || run.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("run = %+v", run)
	}
}

func TestRunsCancelRequiresConfirmationAndReturnsRootSnapshot(t *testing.T) {
	runtime := instantRuntime()
	runtime.Instant = false
	runtime.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{
			Delay: time.Hour,
			Event: agent.RunFinished{Outcome: agent.Outcome{Status: protocol.OutcomeCompleted}},
		}}}
	}
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: "ses_demo_1", Message: agent.Message{Text: "keep running"},
		Options: agent.RunOptions{Limits: agent.UnlimitedRunLimits()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, executeCommandErr := executeCommand(t, runtime, "", "runs", "cancel", opened.RunID); executeCommandErr == nil {
		t.Fatal("runs cancel did not require --yes")
	}
	stillRunning, err := runtime.GetRun(t.Context(), opened.RunID)
	if err != nil || stillRunning.Status != protocol.RunStatusRunning {
		t.Fatalf("unconfirmed cancel changed run = %+v, %v", stillRunning, err)
	}

	out, _, err := executeCommand(t, runtime, "", "runs", "cancel", opened.RunID, "--yes", "--reason", "operator stopped it", "--json")
	if err != nil {
		t.Fatalf("runs cancel: %v", err)
	}
	var result protocol.CancelRunResponse
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("runs cancel output: %v\n%s", err, out)
	}
	if result.Run.ID != opened.RunID || result.Run.Outcome.Type != "canceled" ||
		result.Run.Outcome.Detail != "operator stopped it" || result.Type != protocol.CancelRunRoot || result.RootRun != nil || result.Run.Status != protocol.RunStatusFinished {
		t.Fatalf("result = %+v", result)
	}
}

type childCancellationRuntime struct {
	Runtime
	result protocol.CancelRunResponse
}

type uncertainRunCancellationRuntime struct {
	Runtime

	mu       sync.Mutex
	attempts []agent.CancelRun
}

func (u *uncertainRunCancellationRuntime) CancelRun(ctx context.Context, request agent.CancelRun) (protocol.CancelRunResponse, error) {
	u.mu.Lock()
	u.attempts = append(u.attempts, request)
	attempt := len(u.attempts)
	u.mu.Unlock()
	if attempt == 1 {
		return protocol.CancelRunResponse{}, fmt.Errorf("cancellation acknowledgement timed out: %w", context.DeadlineExceeded)
	}
	return u.Runtime.CancelRun(ctx, request)
}

func (u *uncertainRunCancellationRuntime) cancelAttempts() []agent.CancelRun {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]agent.CancelRun(nil), u.attempts...)
}

func (c childCancellationRuntime) CancelRun(context.Context, agent.CancelRun) (protocol.CancelRunResponse, error) {
	return c.result, nil
}

func TestRunsCancelPreservesSurvivingRootStateForAChild(t *testing.T) {
	lineage := protocol.RunSummary{ID: "run_child", SpawnedByItemID: "item_spawn", ParentRunID: "run_root", RootRunID: "run_root"}
	child := protocol.RunRef{RunSummary: protocol.RunSummary{ID: "run_child", SessionID: "ses_1", SpawnedByItemID: (lineage).SpawnedByItemID, ParentRunID: (lineage).ParentRunID, RootRunID: (lineage).RootRunID, Status: protocol.RunStatusFinished, Outcome: (agent.Outcome{Status: protocol.OutcomeCanceled}).RunOutcome()}}
	root := protocol.RunRef{RunSummary: protocol.RunSummary{ID: "run_root", SessionID: "ses_1", Status: protocol.RunStatusWaiting}}
	runtime := childCancellationRuntime{
		Runtime: instantRuntime(),
		result:  protocol.CancelRunResponse{Type: protocol.CancelRunChild, Run: child, RootRun: &root},
	}
	out, _, err := executeCommand(t, runtime, "", "runs", "cancel", "run_child", "--yes", "--json")
	if err != nil {
		t.Fatalf("runs cancel child: %v", err)
	}
	if !strings.Contains(out, `"id":"run_child"`) || !strings.Contains(out, `"id":"run_root"`) || !strings.Contains(out, `"status":"waiting"`) {
		t.Fatalf("child cancellation output omitted surviving root:\n%s", out)
	}
}

func TestRunsCancelConfirmsTimeoutWithOneMutationIdentity(t *testing.T) {
	base := instantRuntime()
	base.Instant = false
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{
			Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: protocol.OutcomeCompleted}},
		}}}
	}
	opened, err := base.StartRun(t.Context(), agent.StartRun{
		SessionID: "ses_demo_1", Message: agent.Message{Text: "cancel through subcommand"},
		Options: agent.RunOptions{Limits: agent.UnlimitedRunLimits()},
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &uncertainRunCancellationRuntime{Runtime: base}
	profile := commandRuntimeProfile(t)
	if _, _, err := executeCommandWithRuntime(t, runtime, &profile, "", "runs", "cancel", opened.RunID, "--yes"); err != nil {
		t.Fatal(err)
	}
	attempts := runtime.cancelAttempts()
	if len(attempts) != 2 || attempts[0].CommandID == "" || attempts[0].CommandID != attempts[1].CommandID ||
		attempts[0].RunID != opened.RunID || attempts[1].RunID != opened.RunID {
		t.Fatalf("cancellation confirmation attempts = %+v", attempts)
	}
}

func TestRunIDCompletionIncludesDescendants(t *testing.T) {
	runtime := &recordingRunCatalog{Runtime: instantRuntime()}
	out, _, err := executeCommand(t, runtime, "", "__complete", "runs", "show", "run_demo")
	if err != nil || !strings.Contains(out, "run_demo_history") {
		t.Fatalf("completion = %q, %v", out, err)
	}
	if len(runtime.queries) != 1 {
		t.Fatalf("completion queries = %+v", runtime.queries)
	}
	rows, rowsErr := runtime.queries[0].PageSize.Rows()
	if !runtime.queries[0].IncludeDescendants || rowsErr != nil || rows != agent.MaximumPageRows {
		t.Fatalf("completion query = %+v", runtime.queries)
	}
}

func TestRunIDCompletionFallsBackToRootsWithoutSubagents(t *testing.T) {
	t.Parallel()
	profile := commandRuntimeProfile(t, func(discovery *protocol.DiscoverResponse, client *protocol.ClientCapabilities) {
		discovery.Capabilities.Features[protocol.FeatureSubagents] = protocol.FeatureCapability{
			ClientOptIn: true,
		}
	})
	runtime := &recordingRunCatalog{Runtime: instantRuntime()}
	provider := runtimeProvider{open: func(context.Context) (Runtime, *runtimebinding.Profile, error) {
		return runtime, new(profile), nil
	}}
	command := newRunsShowCommand(provider)
	command.SetContext(t.Context())
	items, directive := command.ValidArgsFunction(command, nil, "run_demo")
	if directive != cobra.ShellCompDirectiveNoFileComp || len(items) == 0 {
		t.Fatalf("completion = (%v, %v)", items, directive)
	}
	if len(runtime.queries) != 1 {
		t.Fatalf("completion queries = %+v", runtime.queries)
	}
	rows, rowsErr := runtime.queries[0].PageSize.Rows()
	if runtime.queries[0].IncludeDescendants || rowsErr != nil || rows != agent.MaximumPageRows {
		t.Fatalf("completion query = %+v", runtime.queries)
	}
}
