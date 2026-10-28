package execution

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type steerReadCancellation struct {
	runs.ExecutionTreeStore
	caller context.Context
	cancel context.CancelFunc
}

func (s steerReadCancellation) LoadExecutionTree(ctx context.Context, sessionID, rootID string) (runs.ExecutionTreeHead, bool, error) {
	head, found, err := s.ExecutionTreeStore.LoadExecutionTree(ctx, sessionID, rootID)
	if ctx == s.caller && found && err == nil {
		s.cancel()
	}
	return head, found, err
}

func TestUnadmittedSteerSurvivesUncertainExecutionRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := interactionTestStart()
		start.CWD = t.TempDir()
		start.WorkspaceCWD = start.CWD
		input := []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "apply once"}}
		prepared := runs.PreparedSteer{ItemID: "item_original", Input: input, Acceptance: &runs.CommandAcceptance{
			Key: "original-key", Fingerprint: strings.Repeat("a", 64), Operation: runs.CommandSteerInput,
			SessionID: start.SessionID, RunID: "run_original", SegmentID: "seg_original",
			UserItemID: "item_original", SteerInput: transcript.CloneContent(input),
		}}
		original := prepared.Clone()
		caller, cancel := context.WithCancel(t.Context())
		defer cancel()
		trees := steerReadCancellation{ExecutionTreeStore: testTrees(t), caller: caller, cancel: cancel}

		installation := testsupport.InstallationID(t)
		source, err := mcpserver.InstallationSource(installation, testsupport.Digest("steer recovery release"), testsupport.Digest("steer recovery authority"), testsupport.Digest("steer recovery recipient"))
		if err != nil {
			t.Fatal(err)
		}
		configuration := mcp.ServerConfig{Server: mcpserver.Server{Source: source, Name: testsupport.ServerName("schedules"), Transport: mcpserver.TransportStdio, Command: "schedule-test", Enabled: true}}
		var toolCalls, modelCalls atomic.Int32
		inner, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "schedules_append_record", Description: "Append one record."}, func(context.Context, struct{}) (string, error) {
			toolCalls.Add(1)
			return "", errors.New("reply lost after append")
		})
		if err != nil {
			t.Fatal(err)
		}
		modelEntered, modelRelease := make(chan struct{}), make(chan struct{})
		releaseModel := sync.OnceFunc(func() { close(modelRelease) })
		defer releaseModel()
		model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
			if modelCalls.Add(1) == 1 {
				close(modelEntered)
				select {
				case <-modelRelease:
					return interactionToolResponse(chat.ToolCall{ID: "append", Name: "schedules_append_record", Arguments: `{}`}, 1, 1), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return interactionUsageTextResponse("completed", 1, 1), nil
		})
		probeEntered, probeRelease := make(chan struct{}), make(chan struct{})
		releaseProbe := sync.OnceFunc(func() { close(probeRelease) })
		defer releaseProbe()
		probe := &scheduleReceiptProbe{owned: true, query: func(ctx context.Context) (*chat.ToolOutput, error) {
			close(probeEntered)
			select {
			case <-probeRelease:
				output := chat.NewTextToolOutput("original record")
				return &output, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}}
		config := InteractionExecutorConfig{
			ExecutionTrees: trees, OriginalToolResults: probe,
			ToolResolver: staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{
				scheduleReceiptTool{Tool: inner, configuration: configuration, remote: "append_record"},
			}}},
			ToolInterpreter: immutableToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
		}
		executor := newObservedTestInteractionExecutor(t, model, config)
		ref, err := executor.StageRoot(t.Context(), start)
		if err != nil {
			t.Fatal(err)
		}
		live, err := executor.session(ref)
		if err != nil {
			t.Fatal(err)
		}
		live.lifetime.stopReconciling()
		sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
		if err != nil {
			t.Fatal(err)
		}
		preparedCalls := make(chan runs.ToolCallPrepared, 1)
		originalEvents := collectInteractionEvents(func(yield func(runs.ExecutorEvent) bool) {
			for event := range sequence {
				if commit, ok := event.Payload.(runs.ExecutionFactCommit); ok {
					if call, ok := commit.Fact().(runs.ToolCallPrepared); ok {
						preparedCalls <- call
					}
				}
				if !yield(event) {
					return
				}
			}
		})
		if err := executor.BeginRoot(t.Context(), ref); err != nil {
			t.Fatal(err)
		}
		<-modelEntered
		if _, err := executor.SubmitSteer(caller, ref, prepared); !errors.Is(err, context.Canceled) {
			t.Fatalf("steer cancellation before admission: %v", err)
		}
		prepared.Input[0].Text = "caller changed input"
		prepared.Acceptance.SteerInput[0].Text = "caller changed acceptance"
		prepared.Acceptance.Key = "changed-key"
		prepared.Acceptance.Fingerprint = strings.Repeat("b", 64)
		prepared.Acceptance.SegmentID = "seg_changed"
		releaseModel()
		synctest.Wait()
		var call runs.ToolCallPrepared
		select {
		case call = <-preparedCalls:
		default:
			t.Fatal("installed Tool never captured its pre-call checkpoint")
		}
		ids, err := live.unknownEffectIDs(t.Context())
		if err != nil || len(ids) != 1 {
			t.Fatalf("unknown Tool effects: %v, %v", ids, err)
		}
		tree, err := live.committedTree(t.Context(), live.state.processHandle().Relation().RootID())
		if err != nil {
			t.Fatal(err)
		}
		accepted, err := live.admittedSteers(tree)
		if err != nil || len(accepted) != 0 {
			t.Fatalf("canceled caller created an acceptance: %+v, %v", accepted, err)
		}
		if err := executor.Release(t.Context(), ref); err != nil {
			t.Fatal(err)
		}
		<-originalEvents

		at := time.Now().UTC()
		arguments, err := domaintool.ParseArguments(call.Start.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		item, err := transcript.NewToolCall(transcript.ItemIdentity{
			SessionID: start.SessionID, RunID: original.Acceptance.RunID, ItemID: "item_original_tool", OccurredAt: at,
		}, transcript.ToolInvocation{Name: call.Start.ToolName, Arguments: arguments}, call.Start.SafetyClass)
		if err != nil {
			t.Fatal(err)
		}
		active := runs.ActiveExecution{
			Run: testsupport.MustRestoreRun(run.Snapshot{
				ID: original.Acceptance.RunID, SessionID: start.SessionID, State: run.Running,
				ActiveSegmentID: original.Acceptance.SegmentID, ModelSelection: start.ModelSelection,
				Metrics: rootRunMetricsForCheckpoint(call.Context),
			}),
			Session: testsupport.MustRestoreSession(session.Snapshot{ID: start.SessionID, Workspace: testsupport.MustWorkspace(start.WorkspaceCWD), Selection: start.ModelSelection}),
			Context: call.Context,
			Tools: []runs.OpenToolInvocation{{
				SessionID: start.SessionID, RunID: original.Acceptance.RunID, SegmentID: original.Acceptance.SegmentID,
				CallID: call.Start.CallID, ItemID: item.ID(), StartedAt: at,
			}},
			Items: []transcript.Item{item},
		}
		restarted := newObservedTestInteractionExecutor(t, model, config)
		plan, eligible, err := restarted.PrepareUncertainExecution(t.Context(), active)
		if err != nil || !eligible {
			t.Fatalf("original execution cannot recover: eligible=%t, %v", eligible, err)
		}
		restoredRef, err := restarted.StageUncertainExecution(t.Context(), plan.Continuation)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := restarted.session(restoredRef)
		if err != nil {
			t.Fatal(err)
		}
		sequence, err = observeTestInteraction(t, restarted, t.Context(), restoredRef)
		if err != nil {
			t.Fatal(err)
		}
		restoredEvents := collectInteractionEvents(sequence)
		begun := make(chan error, 1)
		go func() { begun <- restarted.BeginUncertainExecution(t.Context(), restoredRef) }()
		select {
		case <-probeEntered:
		case err := <-begun:
			t.Fatalf("recovery did not query the original Tool: %v", err)
		}
		tree, err = restored.committedTree(t.Context(), restored.state.processHandle().Relation().RootID())
		if err != nil {
			t.Fatal(err)
		}
		accepted, err = restored.admittedSteers(tree)
		if err != nil || len(accepted) != 0 {
			t.Fatalf("checkpoint restoration accepted the pending command: %+v, %v", accepted, err)
		}
		itemID, err := restarted.SubmitSteer(t.Context(), restoredRef, original)
		if err != nil || itemID != original.ItemID {
			t.Fatalf("same command in original Segment: %q, %v", itemID, err)
		}
		conflicting := original.Clone()
		conflicting.Input[0].Text = "different input"
		conflicting.Acceptance.SteerInput = transcript.CloneContent(conflicting.Input)
		conflicting.Acceptance.Fingerprint = strings.Repeat("c", 64)
		if _, err := restarted.SubmitSteer(t.Context(), restoredRef, conflicting); !errors.Is(err, runs.ErrCommandKeyConflict) {
			t.Fatalf("changed input reused the original key: %v", err)
		}
		tree, err = restored.committedTree(t.Context(), restored.state.processHandle().Relation().RootID())
		if err != nil {
			t.Fatal(err)
		}
		accepted, err = restored.admittedSteers(tree)
		if err != nil || len(accepted) != 1 || !accepted[0].Equal(*original.Acceptance) {
			t.Fatalf("retried admission changed its original identity: %+v, %v", accepted, err)
		}
		releaseProbe()
		if err := <-begun; err != nil {
			t.Fatal(err)
		}
		observed := <-restoredEvents
		applied := payloadsOf[runs.SteerMessagesApplied](observed)
		if len(applied) != 1 || len(applied[0].Messages) != 1 || applied[0].Messages[0].ItemID != original.ItemID ||
			!slices.EqualFunc(applied[0].Messages[0].Content, original.Input, transcript.ContentBlock.Equal) {
			t.Fatalf("restored command did not apply its original input once: %+v", applied)
		}
		ends := payloadsOf[runs.SegmentEnded](observed)
		if len(ends) != 1 || ends[0].Reason != run.OutcomeCompleted || toolCalls.Load() != 1 || modelCalls.Load() != 2 {
			t.Fatalf("recovery redispatched or failed: ends=%+v, Tool calls=%d, model calls=%d", ends, toolCalls.Load(), modelCalls.Load())
		}
	})
}
