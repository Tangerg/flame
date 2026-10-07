package terminal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	runworkflow "github.com/Tangerg/flame/cli/internal/application/agent/run"
	"github.com/Tangerg/flame/cli/internal/application/mutation"
	"github.com/Tangerg/flame/cli/internal/application/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/core/input"
)

func TestSteerReceiptRequiresTheExactCompletedUserItem(t *testing.T) {
	for _, itemBeforeACK := range []bool{false, true} {
		name := "ack before item"
		if itemBeforeACK {
			name = "item before ack"
		}
		t.Run(name, func(t *testing.T) {
			var receipts steerReceipts
			result := receiptTestResult(t, "ses_1", "run_1", "seg_1", "item_exact")
			entry := receipts.track(result.Pending)
			if !itemBeforeACK {
				receipts.accept(result)
			}
			for _, block := range []conversation.Block{
				{ID: "item_other", RunID: "run_1", Kind: conversation.BlockUser, Status: conversation.BlockStatusCompleted, Text: "same text"},
				{ID: "item_exact", RunID: "run_other", Kind: conversation.BlockUser, Status: conversation.BlockStatusCompleted},
				{ID: "item_exact", RunID: "run_1", Kind: conversation.BlockAssistant, Status: conversation.BlockStatusCompleted},
				{ID: "item_exact", RunID: "run_1", Kind: conversation.BlockUser, Status: conversation.BlockStatusRunning},
			} {
				entry.observeBlock(block)
			}
			if got := entry.status(); got == steerApplied {
				t.Fatalf("unrelated content claimed applied: %s", got)
			}
			entry.observeBlock(conversation.Block{ID: "item_exact", RunID: "run_1", Kind: conversation.BlockUser, Status: conversation.BlockStatusCompleted})
			if itemBeforeACK {
				if got := entry.status(); got != "" {
					t.Fatalf("unacknowledged item claimed steer: %s", got)
				}
				receipts.accept(result)
			}
			if got := entry.status(); got != steerApplied {
				t.Fatalf("exact durable item status = %s", got)
			}
		})
	}
}

func TestSteerReceiptRetainsAnAuthoritativeReadBeforeACK(t *testing.T) {
	for _, itemApplied := range []bool{false, true} {
		name := "absent item"
		want := steerNotApplied
		if itemApplied {
			name, want = "durable item", steerApplied
		}
		t.Run(name, func(t *testing.T) {
			var receipts steerReceipts
			result := receiptTestResult(t, "ses_1", "run_1", "seg_1", "item_exact")
			entry := receipts.track(result.Pending)
			snapshot := conversation.SessionSnapshot{
				Session: conversation.Session{ID: "ses_1"},
				Runs:    []conversation.Run{{ID: "run_1", Status: protocol.RunStatusFinished}},
			}
			if itemApplied {
				snapshot.Transcript = []conversation.Block{{ID: "item_exact", RunID: "run_1", Kind: conversation.BlockUser, Status: conversation.BlockStatusCompleted}}
			}
			receipts.observeSnapshot(snapshot)
			if got := entry.status(); got != steerUnconfirmed {
				t.Fatalf("terminal read without a receipt = %s", got)
			}
			receipts.accept(result)
			if got := entry.status(); got != want {
				t.Fatalf("late ack status = %s, want %s", got, want)
			}
		})
	}
}

func TestSteerReceiptCanApplyAfterAnInterruptedSegment(t *testing.T) {
	var receipts steerReceipts
	result := receiptTestResult(t, "ses_1", "run_root", "seg_root", "item_exact")
	receipts.accept(result)
	entry := receipts.entries[0]
	receipts.observeEvent("ses_1", conversation.RunEvent{RunID: "run_child", SegmentID: "seg_child", Event: conversation.SegmentFinished{
		Run: conversation.Run{ID: "run_child", Status: protocol.RunStatusWaiting},
	}})
	if got := entry.status(); got != steerAccepted || len(receipts.needingRead("ses_1")) != 0 {
		t.Fatalf("member interrupt settled root receipt: %s", got)
	}
	receipts.observeEvent("ses_1", conversation.RunEvent{RunID: "run_root", SegmentID: "seg_root", Event: conversation.SegmentFinished{
		Run: conversation.Run{ID: "run_root", Status: protocol.RunStatusWaiting},
	}})
	if got := entry.status(); got != steerAccepted || len(receipts.needingRead("ses_1")) != 0 {
		t.Fatalf("paused run without item = %s", got)
	}
	receipts.observeSnapshot(conversation.SessionSnapshot{
		Session: conversation.Session{ID: "ses_1"}, Runs: []conversation.Run{{ID: "run_root", Status: protocol.RunStatusWaiting}},
	})
	if got := entry.status(); got != steerAccepted {
		t.Fatalf("authoritative waiting snapshot = %s", got)
	}
	receipts.observeEvent("ses_1", conversation.RunEvent{
		RunID: "run_root", SegmentID: "seg_continuation",
		Event: conversation.BlockCompleted{Block: conversation.Block{
			ID: "item_exact", RunID: "run_root", Kind: conversation.BlockUser, Status: conversation.BlockStatusCompleted,
		}},
	})
	if got := entry.status(); got != steerApplied {
		t.Fatalf("steer applied in the same run's next segment = %s", got)
	}
}

func TestRecoveredSteerReceiptsKeepTheirSessionAndRunIdentities(t *testing.T) {
	var receipts steerReceipts
	first := receiptTestResult(t, "ses_1", "run_1", "seg_1", "item_1")
	second := receiptTestResult(t, "ses_2", "run_2", "seg_2", "item_2")
	receipts.accept(first)
	receipts.accept(second)
	receipts.observeSnapshot(conversation.SessionSnapshot{
		Session: conversation.Session{ID: "ses_1"}, Runs: []conversation.Run{{ID: "run_1", Status: protocol.RunStatusFinished}},
		Transcript: []conversation.Block{{ID: "item_2", RunID: "run_2", Kind: conversation.BlockUser, Status: conversation.BlockStatusCompleted}},
	})
	if got := receipts.entries[0].status(); got != steerNotApplied {
		t.Fatalf("first session receipt = %s", got)
	}
	if got := receipts.entries[1].status(); got != steerAccepted {
		t.Fatalf("another session changed the second receipt = %s", got)
	}
	receipts.observeSnapshot(conversation.SessionSnapshot{
		Session: conversation.Session{ID: "ses_2"}, Runs: []conversation.Run{{ID: "run_2", Status: protocol.RunStatusFinished}},
		Transcript: []conversation.Block{{ID: "item_2", RunID: "run_2", Kind: conversation.BlockUser, Status: conversation.BlockStatusCompleted}},
	})
	if got := receipts.entries[1].status(); got != steerApplied {
		t.Fatalf("second session restored receipt = %s", got)
	}
}

func TestSteerPresentationKeepsAnUnappliedInstructionVisibleBesideAppliedInput(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		for _, missingFirst := range []bool{false, true} {
			var receipts steerReceipts
			missing := receiptTestResult(t, "ses_1", "run_1", "seg_1", "item_missing")
			if unknown {
				receipts.track(missing.Pending)
			} else {
				receipts.accept(missing)
			}
			command := missing.Pending.Command()
			command.CommandID = "cli_66666666666666666666666666666666"
			pending, err := workbench.NewPendingSteer("ses_1", command, missing.Pending.StagedAt(), missing.Pending.Replay())
			if err != nil {
				t.Fatal(err)
			}
			receipts.accept(runworkflow.SteerResult{
				Pending: pending, Outcome: mutation.Confirmed, Receipt: protocol.SteerRunResponse{UserItemID: "item_applied"},
			})
			if !missingFirst {
				receipts.entries[0], receipts.entries[1] = receipts.entries[1], receipts.entries[0]
			}
			receipts.observeSnapshot(conversation.SessionSnapshot{
				Session: conversation.Session{ID: "ses_1"}, Runs: []conversation.Run{{ID: "run_1", Status: protocol.RunStatusFinished}},
				Transcript: []conversation.Block{{ID: "item_applied", RunID: "run_1", Kind: conversation.BlockUser, Status: conversation.BlockStatusCompleted}},
			})
			terminal := app{
				session:   sessionState{current: conversation.Session{ID: "ses_1"}},
				execution: executionState{conversation: conversation.New()}, status: &statusView{}, steers: receipts,
			}
			terminal.presentSteerReceipts()
			want := "steer not applied"
			if unknown {
				want = "steer application remains unconfirmed"
			}
			if !strings.Contains(terminal.status.doing, want) {
				t.Fatalf("unknown=%t missingFirst=%t status=%q", unknown, missingFirst, terminal.status.doing)
			}
		}
	}
}

func TestLateSteerACKCannotHideAnotherUnappliedInstructionInTheSameRun(t *testing.T) {
	var receipts steerReceipts
	late := receiptTestResult(t, "ses_1", "run_1", "seg_1", "item_applied")
	receipts.track(late.Pending)
	command := late.Pending.Command()
	command.CommandID = "cli_66666666666666666666666666666666"
	pending, err := workbench.NewPendingSteer("ses_1", command, late.Pending.StagedAt(), late.Pending.Replay())
	if err != nil {
		t.Fatal(err)
	}
	receipts.accept(runworkflow.SteerResult{
		Pending: pending, Outcome: mutation.Confirmed, Receipt: protocol.SteerRunResponse{UserItemID: "item_missing"},
	})
	receipts.observeSnapshot(conversation.SessionSnapshot{
		Session: conversation.Session{ID: "ses_1"}, Runs: []conversation.Run{{ID: "run_1", Status: protocol.RunStatusFinished}},
		Transcript: []conversation.Block{{ID: "item_applied", RunID: "run_1", Kind: conversation.BlockUser, Status: conversation.BlockStatusCompleted}},
	})
	terminal := app{
		session:   sessionState{current: conversation.Session{ID: "ses_1"}},
		execution: executionState{conversation: conversation.New()}, status: &statusView{}, steers: receipts,
	}
	terminal.presentSteerReceipts()
	if !strings.Contains(terminal.status.doing, "steer not applied") || !strings.Contains(terminal.status.doing, "item_missing") {
		t.Fatalf("completed run steer status = %q", terminal.status.doing)
	}
	terminal.steers.accept(late)
	terminal.presentSteerReceipts()
	if !strings.Contains(terminal.status.doing, "steer not applied") || !strings.Contains(terminal.status.doing, "item_missing") {
		t.Fatalf("late applied acknowledgement hid the unapplied instruction: %q", terminal.status.doing)
	}
}

func receiptTestResult(t *testing.T, sessionID, runID, segmentID, itemID string) runworkflow.SteerResult {
	t.Helper()
	pending, err := workbench.NewPendingSteer(sessionID, prompt.SteerRun{
		CommandID: "cli_55555555555555555555555555555555", RunID: runID, SegmentID: segmentID,
		Message: prompt.Message{Text: "same text"},
	}, time.Date(2026, 9, 26, 1, 0, 0, 0, time.UTC), replay.UnprotectedGuard())
	if err != nil {
		t.Fatal(err)
	}
	return runworkflow.SteerResult{Pending: pending, Outcome: mutation.Confirmed, Receipt: protocol.SteerRunResponse{UserItemID: itemID}}
}

type delayedSteerReceiptRuntime struct {
	*runtimefixture.Runtime
	mu          sync.Mutex
	target      string
	applyInput  bool
	requests    chan prompt.SteerRun
	acknowledge chan struct{}
	read        chan struct{}
	allowRead   chan struct{}
	readError   error
}

func (r *delayedSteerReceiptRuntime) SteerRun(ctx context.Context, request prompt.SteerRun) (protocol.SteerRunResponse, error) {
	r.mu.Lock()
	r.target = request.RunID
	r.mu.Unlock()
	receipt := protocol.SteerRunResponse{UserItemID: "item_reserved"}
	if r.applyInput {
		var err error
		receipt, err = r.Runtime.SteerRun(ctx, request)
		if err != nil {
			return protocol.SteerRunResponse{}, err
		}
	}
	select {
	case r.requests <- request:
	case <-ctx.Done():
		return protocol.SteerRunResponse{}, ctx.Err()
	}
	select {
	case <-r.acknowledge:
		return receipt, nil
	case <-ctx.Done():
		return protocol.SteerRunResponse{}, ctx.Err()
	}
}

func (r *delayedSteerReceiptRuntime) GetSession(ctx context.Context, sessionID string) (conversation.SessionSnapshot, error) {
	snapshot, err := r.Runtime.GetSession(ctx, sessionID)
	if err != nil {
		return snapshot, err
	}
	r.mu.Lock()
	target := r.target
	r.mu.Unlock()
	for _, run := range snapshot.Runs {
		if run.ID != target || run.Status != protocol.RunStatusFinished {
			continue
		}
		select {
		case r.read <- struct{}{}:
		default:
		}
		if r.allowRead != nil {
			select {
			case <-r.allowRead:
			case <-ctx.Done():
				return conversation.SessionSnapshot{}, ctx.Err()
			}
		}
		return snapshot, r.readError
	}
	return snapshot, nil
}

func TestTerminalSteerSettlesAReadThatArrivesBeforeACK(t *testing.T) {
	for _, applied := range []bool{false, true} {
		name, label := "not applied", "steer not applied to this run"
		if applied {
			name, label = "applied", "steer applied to model context"
		}
		t.Run(name, func(t *testing.T) {
			backend := delayedSteerRuntime(t)
			backend.applyInput = applied
			host, stop := runUIWithWorkspace(t, backend, t.TempDir())
			host.Shows(t, "Ask flame")
			host.Type("work until canceled")
			host.Press(input.Enter)
			host.Shows(t, "thinking")
			host.Type("/steer same text")
			host.Press(input.Enter)
			request := awaitSignalValue(t, backend.requests, "steer admission")
			if _, err := backend.Runtime.CancelRun(t.Context(), conversation.CancelRun{RunID: request.RunID}); err != nil {
				t.Fatal(err)
			}
			awaitSignalValue(t, backend.read, "authoritative read after segment completion")
			host.Shows(t, "steer application remains unconfirmed")
			close(backend.acknowledge)
			host.Shows(t, label)
			page, err := backend.Runtime.ListRuns(t.Context(), conversation.RunQuery{
				SessionID: firstRuntimeSession(t, backend.Runtime), PageSize: conversation.DefaultPageSize(),
			})
			if err != nil || len(page.Items) != 1 {
				t.Fatalf("steer settlement started another run: %+v, %v", page, err)
			}
			stop()
		})
	}
}

func TestTerminalSteerWaitsForTheAuthoritativeReadAndReportsReadFailure(t *testing.T) {
	for _, failure := range []error{nil, conversation.ErrIncompatibleRuntime} {
		name, label := "missing item", "steer not applied to this run"
		if failure != nil {
			name, label = "read failed", "steer application remains unconfirmed"
		}
		t.Run(name, func(t *testing.T) {
			backend := delayedSteerRuntime(t)
			backend.allowRead = make(chan struct{})
			backend.readError = failure
			host, stop := runUIWithWorkspace(t, backend, t.TempDir())
			host.Shows(t, "Ask flame")
			host.Type("work until canceled")
			host.Press(input.Enter)
			host.Shows(t, "thinking")
			host.Type("/steer same text")
			host.Press(input.Enter)
			request := awaitSignalValue(t, backend.requests, "steer admission")
			close(backend.acknowledge)
			host.Shows(t, "steer accepted")
			host.Hides(t, "steer applied")
			if _, err := backend.Runtime.CancelRun(t.Context(), conversation.CancelRun{RunID: request.RunID}); err != nil {
				t.Fatal(err)
			}
			awaitSignalValue(t, backend.read, "authoritative read after segment completion")
			host.Shows(t, "verifying application")
			host.Hides(t, "steer not applied")
			close(backend.allowRead)
			host.Shows(t, label)
			if errors.Is(failure, conversation.ErrIncompatibleRuntime) {
				host.Hides(t, "steer not applied")
			}
			stop()
		})
	}
}

func TestTerminalDoesNotReplayOldSteerStatusAfterALaterRun(t *testing.T) {
	backend := delayedSteerRuntime(t)
	backend.applyInput = true
	longWork := backend.Runtime.Script
	backend.Runtime.Script = func(authoredPrompt string) runtimefixture.Script {
		if authoredPrompt == "later work" {
			return runtimefixture.Script{Prelude: []runtimefixture.Step{
				{Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
			}}
		}
		return longWork(authoredPrompt)
	}
	close(backend.acknowledge)
	host, stop := runUIWithWorkspace(t, backend, t.TempDir())
	host.Shows(t, "Ask flame")
	host.Type("work until canceled")
	host.Press(input.Enter)
	host.Shows(t, "thinking")
	host.Type("/steer same text")
	host.Press(input.Enter)
	request := awaitSignalValue(t, backend.requests, "steer admission")
	host.Shows(t, "steer applied to model context")
	if _, err := backend.Runtime.CancelRun(t.Context(), conversation.CancelRun{RunID: request.RunID}); err != nil {
		t.Fatal(err)
	}
	host.Type("later work")
	host.Press(input.Enter)
	host.Shows(t, "complete")
	host.Hides(t, "steer applied")
	stop()
}

func delayedSteerRuntime(t *testing.T) *delayedSteerReceiptRuntime {
	t.Helper()
	base := runtimefixture.New()
	base.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{
			{Event: conversation.BlockStarted{Block: conversation.Block{ID: "thinking", Kind: conversation.BlockReasoning}}},
			{Delay: time.Hour, Finish: &runtimefixture.Finish{Outcome: conversation.Outcome{Status: protocol.OutcomeCompleted}}},
		}}
	}
	return &delayedSteerReceiptRuntime{
		Runtime: base, requests: make(chan prompt.SteerRun, 1), acknowledge: make(chan struct{}), read: make(chan struct{}, 1),
	}
}
