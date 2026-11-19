package bootstrap

import (
	"context"
	"database/sql"
	"database/sql/driver"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
	modernsqlite "modernc.org/sqlite"
)

const (
	waitingChildBeforeControl     = "before Scope control"
	waitingChildAfterControl      = "after Scope control"
	waitingChildAfterRelease      = "after Scope release"
	waitingChildRetirementRefused = "cancellation retirement refused"
	waitingChildCutEnv            = "FLAME_TEST_WAITING_CHILD_CANCEL_CUT"
	waitingChildMarkerEnv         = "FLAME_TEST_WAITING_CHILD_CANCEL_MARKER"
)

type waitingChildCancellationMarker struct {
	RootID      string
	ChildID     string
	SegmentID   string
	ChildMember string
}

func waitingChildCancellationMeta() protocol.RequestMeta {
	return protocol.RequestMeta{
		ProtocolVersion: protocol.Version,
		ClientCapabilities: &protocol.ClientCapabilities{
			Features:       map[string]protocol.FeaturePreference{protocol.FeatureChildRuns: {Enabled: true}},
			InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion},
		},
	}
}

func singleWaitingChildModel(continued chan<- struct{}, finish <-chan struct{}) chat.Model {
	return delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		var gotResult, child bool
		for _, message := range request.Messages {
			gotResult = gotResult || message.Role == chat.RoleTool
			child = child || message.Role == chat.RoleUser && strings.Contains(message.Text(), "ask before acting")
		}
		message := chat.NewAssistantMessage(chat.NewTextPart("parent continued after the child was canceled"))
		reason := chat.FinishReasonStop
		switch {
		case len(request.Tools) == 0:
		case gotResult:
			if !child && continued != nil {
				close(continued)
				select {
				case <-finish:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		case child:
			message = chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
				ID: "ask", Name: "ask_user", Arguments: `{"questions":[{"question":"Continue this child?"}]}`,
			}))
			reason = chat.FinishReasonToolCalls
		default:
			message = chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
				ID: "delegate", Name: "delegate_task", Arguments: `{"summary":"One child action","instructions":"ask before acting"}`,
			}))
			reason = chat.FinishReasonToolCalls
		}
		return chat.NewResponse(&chat.Output{Message: &message, FinishReason: reason}, &chat.ResponseMetadata{
			Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1},
		})
	})}
}

func TestWaitingChildCancellationRecoveryHelper(t *testing.T) {
	cut := os.Getenv(waitingChildCutEnv)
	if cut == "" {
		return
	}
	var marker waitingChildCancellationMarker
	if err := modernsqlite.RegisterScalarFunction("test_crash_waiting_child_cancel", 2,
		func(_ *modernsqlite.FunctionContext, arguments []driver.Value) (driver.Value, error) {
			if cut == waitingChildAfterControl {
				log, err := waitingChildScopeLog(arguments[0].([]byte))
				if err != nil {
					return nil, err
				}
				if waitingChildHostControls(log, marker.ChildMember) == 0 {
					return nil, nil
				}
			}
			committed := marker
			committed.SegmentID = arguments[1].(string)
			data, err := json.Marshal(committed)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(os.Getenv(waitingChildMarkerEnv), data, 0o600); err != nil {
				return nil, err
			}
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}

	home := os.Getenv("FLAME_HOME")
	var continued, finish chan struct{}
	afterRelease := cut == waitingChildAfterRelease || cut == waitingChildRetirementRefused
	if afterRelease {
		continued, finish = make(chan struct{}), make(chan struct{})
	}
	host, api, stores := openProtocolRuntimeWithStores(t, singleWaitingChildModel(continued, finish))
	defer host.Close()
	ctx := delivery.WithRequestMeta(t.Context(), waitingChildCancellationMeta())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "cancel the last waiting child",
	})
	if err != nil {
		t.Fatal(err)
	}
	opening, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "delegate one question"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "child question")
	pending, found, err := stores.Interrupts.Get(ctx, opening.RunID)
	if err != nil || !found || len(pending.Bindings) != 1 {
		t.Fatalf("expected the original child question: %+v, %v", pending, err)
	}
	marker.RootID = opening.RunID
	marker.ChildMember = pending.Bindings[0].MemberID
	for _, member := range pending.Continuations {
		if member.MemberID == marker.ChildMember {
			marker.ChildID = member.RunID
			break
		}
	}
	if marker.ChildID == "" || marker.ChildID == marker.RootID {
		t.Fatalf("question did not belong to the child: %+v", marker)
	}
	if afterRelease {
		if cut == waitingChildRetirementRefused {
			questionCrashDatabase(t, home, `CREATE TRIGGER waiting_child_cancel_retirement BEFORE DELETE ON accepted_continuations
				WHEN OLD.canceled_run_id IS NOT NULL
				BEGIN SELECT RAISE(ABORT, 'test refused cancellation retirement'); END`)
		}
		result, err := api.CancelRun(ctx, protocol.CancelRunRequest{RunID: marker.ChildID, Reason: "finish without the child"})
		switch {
		case cut == waitingChildRetirementRefused:
			if settlement, ok := errors.AsType[*runs.CancellationSettlementError](err); !ok || !strings.Contains(settlement.Error(), "refused cancellation retirement") {
				t.Fatalf("the original retirement write was not refused: %+v, %v", result, err)
			}
		case err != nil || result.RootRun == nil:
			t.Fatalf("the cancellation did not release the parent: %+v, %v", result, err)
		}
		accepted, err := stores.AcceptedContinuations.SubtreeCancellations(ctx, marker.RootID)
		if err != nil || (len(accepted) != 0) != (cut == waitingChildRetirementRefused) {
			t.Fatalf("unexpected retained cancellation after Scope release: %+v, %v", accepted, err)
		}
		waitForSignal(t, continued, "released parent's unconfirmed model turn")
		if controls := waitingChildHostControls(readWaitingChildScopeLog(t, home), marker.ChildMember); controls != 1 {
			t.Fatalf("Scope accepted %d cancellation controls for the original child", controls)
		}
		data, err := json.Marshal(marker)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv(waitingChildMarkerEnv), data, 0o600); err != nil {
			t.Fatal(err)
		}
		os.Exit(commandCrashExit)
	}
	questionCrashDatabase(t, home, fmt.Sprintf(`CREATE TRIGGER waiting_child_cancel_crash BEFORE UPDATE ON execution_trees
		WHEN NEW.session_id = '%s' AND EXISTS (SELECT 1 FROM runs WHERE run_id = '%s' AND outcome = 'canceled')
		BEGIN SELECT test_crash_waiting_child_cancel(
			OLD.payload, (SELECT active_segment_id FROM runs WHERE run_id = '%s')
		); END`, session.ID, marker.ChildID, marker.RootID))
	result, err := api.CancelRun(ctx, protocol.CancelRunRequest{RunID: marker.ChildID, Reason: "finish without the child"})
	t.Fatalf("cancellation did not reach the %s cut: %+v, %v", cut, result, err)
}

func TestReleasedChildCancellationLeavesSubsequentModelLossToRunRecovery(t *testing.T) {
	for _, cut := range []string{waitingChildAfterRelease, waitingChildRetirementRefused} {
		t.Run(cut, func(t *testing.T) { testReleasedChildCancellation(t, cut) })
	}
}

func testReleasedChildCancellation(t *testing.T, cut string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	path := filepath.Join(t.TempDir(), "released-child-cancellation.json")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWaitingChildCancellationRecoveryHelper$")
	child.Env = append(os.Environ(), waitingChildCutEnv+"="+cut, waitingChildMarkerEnv+"="+path)
	out, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("cancellation did not release the parent: %v\n%s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var marker waitingChildCancellationMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatal(err)
	}
	if cut == waitingChildRetirementRefused {
		questionCrashDatabase(t, home, "DROP TRIGGER waiting_child_cancel_retirement")
	}
	host, api, stores := openProtocolRuntimeWithStores(t, singleWaitingChildModel(nil, nil))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	runtimeCtx := delivery.WithRequestMeta(t.Context(), waitingChildCancellationMeta())
	parent, err := api.GetRun(runtimeCtx, protocol.GetRunRequest{RunID: marker.RootID})
	if err != nil || parent.Outcome == nil || parent.Outcome.Type != protocol.OutcomeLost {
		t.Fatalf("ordinary Run recovery did not retain the unconfirmed parent result: %+v, %v", parent, err)
	}
	canceled, err := api.GetRun(runtimeCtx, protocol.GetRunRequest{RunID: marker.ChildID})
	if err != nil || canceled.Outcome == nil || canceled.Outcome.Type != protocol.OutcomeCanceled {
		t.Fatalf("parent recovery changed its child's confirmed cancellation: %+v, %v", canceled, err)
	}
	accepted, err := stores.AcceptedContinuations.SubtreeCancellations(runtimeCtx, marker.RootID)
	if err != nil || len(accepted) != 0 {
		t.Fatalf("recovery left a stale cancellation: %+v, %v", accepted, err)
	}
}

func TestWaitingChildCancellationKeepsSurvivingParentAfterProcessCrash(t *testing.T) {
	for _, cut := range []string{waitingChildBeforeControl, waitingChildAfterControl} {
		t.Run(cut, func(t *testing.T) { testWaitingChildCancellationRecovery(t, cut) })
	}
}

func testWaitingChildCancellationRecovery(t *testing.T, cut string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	path := filepath.Join(t.TempDir(), "waiting-child-cancellation.json")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWaitingChildCancellationRecoveryHelper$")
	child.Env = append(os.Environ(), waitingChildCutEnv+"="+cut, waitingChildMarkerEnv+"="+path)
	out, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("cancellation did not reach the %s cut: %v\n%s", cut, err, out)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER waiting_child_cancel_crash")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var marker waitingChildCancellationMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		t.Fatal(err)
	}
	if marker.SegmentID == "" {
		t.Fatal("the cancellation did not commit the original continuation Segment")
	}
	continued, finish := make(chan struct{}), make(chan struct{})
	host, api := openProtocolRuntime(t, singleWaitingChildModel(continued, finish))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	runtimeCtx := delivery.WithRequestMeta(t.Context(), waitingChildCancellationMeta())
	waitForSignal(t, continued, "surviving parent's model turn")
	parent, err := api.GetRun(runtimeCtx, protocol.GetRunRequest{RunID: marker.RootID})
	if err != nil || parent.ActiveSegmentID != marker.SegmentID {
		t.Fatalf("recovery changed the committed parent Segment: %+v, %v; want %s", parent, err, marker.SegmentID)
	}
	canceled, err := api.GetRun(runtimeCtx, protocol.GetRunRequest{RunID: marker.ChildID})
	if err != nil || canceled.Outcome == nil || canceled.Outcome.Type != protocol.OutcomeCanceled {
		t.Fatalf("the original child cancellation did not survive: %+v, %v", canceled, err)
	}
	if controls := waitingChildHostControls(readWaitingChildScopeLog(t, home), marker.ChildMember); controls != 1 {
		t.Fatalf("Scope accepted %d cancellation controls for the original child", controls)
	}
	close(finish)
	waitForProtocolRunTerminal(t, runtimeCtx, api, marker.RootID)
	parent, err = api.GetRun(runtimeCtx, protocol.GetRunRequest{RunID: marker.RootID})
	if err != nil || parent.Outcome == nil || parent.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("the surviving parent did not complete: %+v, %v", parent, err)
	}
}

func waitingChildScopeLog(payload []byte) (scoperun.Log, error) {
	var envelope struct {
		Log scoperun.Log `json:"log"`
	}
	err := json.Unmarshal(payload, &envelope)
	return envelope.Log, err
}

func readWaitingChildScopeLog(t *testing.T, home string) scoperun.Log {
	t.Helper()
	directory, err := localruntime.DataDirectoryAt(home)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", directory.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var data []byte
	if err := db.QueryRowContext(t.Context(), "SELECT payload FROM execution_trees").Scan(&data); err != nil {
		t.Fatal(err)
	}
	log, err := waitingChildScopeLog(data)
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func waitingChildHostControls(log scoperun.Log, memberID string) int {
	var count int
	for _, entry := range log.Entries {
		if control, ok := entry.Record.(scoperun.ControlRequested); ok &&
			control.Kind == scoperun.ControlCancel && control.Origin == scoperun.ControlOriginHost &&
			control.Process.String() == memberID {
			count++
		}
	}
	return count
}
