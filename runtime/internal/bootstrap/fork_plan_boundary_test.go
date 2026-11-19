package bootstrap

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestForkRetainsEveryCopiedRunPlanBoundary(t *testing.T) {
	const earlierStep = "inspect transient paths"
	const latestStep = "review committed state"
	const forkMethod = delivery.Name("sessions.fork")
	home := testsupport.TempDirectory(t)
	t.Setenv("FLAME_HOME", home)

	var nextPlan atomic.Pointer[string]
	var calls atomic.Int32
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		for _, tool := range request.Tools {
			if tool.Name != "set_plan" {
				continue
			}
			step := nextPlan.Swap(nil)
			if step == nil {
				break
			}
			call := chat.ToolCall{
				ID: fmt.Sprintf("plan_%d", calls.Add(1)), Name: tool.Name,
				Arguments: fmt.Sprintf(`{"steps":[{"description":%q,"status":"in_progress"}]}`, *step),
			}
			message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
			return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
		}
		return completedTextResponse("done"), nil
	})
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	author := poMCPActivate(&poMCPRuntime{t: t, host: host, api: api, endpoint: host.Endpoint()}, filepath.Join(firstPartyPackages, "plans"))
	ctx := protocolLifecycleContext(t.Context())
	parent, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "Original Plans",
	})
	if err != nil {
		t.Fatal(err)
	}

	start := func(step string) string {
		t.Helper()
		if step != "" {
			nextPlan.Store(&step)
		}
		result, events, err := api.StartRun(ctx, protocol.StartRunRequest{
			SessionID: parent.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Complete this Plan turn"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		waitForRunEvents(t, collectRunEvents(events), "Plan boundary")
		waitForProtocolRunTerminal(t, ctx, api, result.RunID)
		finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: result.RunID})
		if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted {
			t.Fatalf("Plan Run did not complete: %+v, %v", finished, err)
		}
		if step != "" && nextPlan.Load() != nil {
			t.Fatalf("Run %q did not call the installed Plan author", result.RunID)
		}
		return result.RunID
	}
	start("")
	start(earlierStep)
	original, err := api.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: parent.ID})
	if err != nil || original.Artifact == nil || original.Artifact.PlanSource == nil ||
		original.Artifact.PlanSource.InstallationID != author.ID {
		t.Fatalf("earlier Plan did not retain its author before the fork: %+v, %v", original, err)
	}
	latestRun := start(latestStep)

	expectPlan := func(sessionID, step string) {
		t.Helper()
		current, err := api.GetPlan(ctx, protocol.GetPlanRequest{SessionID: sessionID})
		if err != nil || current == nil || current.State == nil {
			t.Errorf("Session %q lost its recorded Plan boundary: %+v, %v", sessionID, current, err)
			return
		}
		if step == "" {
			if len(current.State.Steps) != 0 {
				t.Errorf("Session %q did not restore the recorded empty Plan: %+v", sessionID, current.State)
			}
			return
		}
		if len(current.State.Steps) != 1 || current.State.Steps[0].Description != step {
			t.Errorf("Session %q Plan = %+v, want %q", sessionID, current.State, step)
		}
	}
	expectPlan(parent.ID, latestStep)
	request := protocol.ForkSessionRequest{SessionID: parent.ID, FromRunID: latestRun, Title: "Copied Plans"}
	meta := protocol.CallMetadata{IdempotencyKey: "copy-historical-plans"}
	fork, err := host.Endpoint().Call[protocol.ForkSessionRequest, *protocol.Session](ctx, forkMethod, request, meta)
	if err != nil {
		t.Fatal(err)
	}
	expectPlan(fork.ID, latestStep)
	if err := api.DeleteSession(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := restarted.Close(); err != nil {
			t.Error(err)
		}
	})
	endpoint := restarted.Endpoint()
	replayed, err := endpoint.Call[protocol.ForkSessionRequest, *protocol.Session](ctx, forkMethod, request, meta)
	if err != nil || replayed == nil || replayed.ID != fork.ID {
		t.Fatalf("copied Session receipt depended on the deleted parent: %+v, %v", replayed, err)
	}
	runs, err := api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: fork.ID})
	if err != nil || len(runs.Data) != 3 {
		t.Fatalf("fork must copy all three Runs: %+v, %v", runs, err)
	}

	branch, err := endpoint.Call[protocol.ForkSessionRequest, *protocol.Session](ctx, forkMethod,
		protocol.ForkSessionRequest{SessionID: fork.ID, FromRunID: runs.Data[1].ID, Title: "Earlier Plan"},
		protocol.CallMetadata{IdempotencyKey: "branch-from-earlier-plan"})
	if err != nil {
		t.Fatal(err)
	}
	expectPlan(branch.ID, earlierStep)
	copied, err := api.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: branch.ID})
	if err != nil || copied.Artifact == nil || copied.Artifact.PlanSource == nil ||
		*copied.Artifact.PlanSource != *original.Artifact.PlanSource {
		t.Fatalf("forked Plan borrowed a different author: %+v, %v", copied, err)
	}
	if _, err := endpoint.Call[protocol.RollbackSessionRequest, *protocol.RollbackSessionResponse](ctx, rollbackCrashMethod,
		protocol.RollbackSessionRequest{SessionID: fork.ID, ToRunID: runs.Data[2].ID},
		protocol.CallMetadata{IdempotencyKey: "restore-recorded-empty-plan"}); err != nil {
		t.Fatal(err)
	}
	expectPlan(fork.ID, "")
}
