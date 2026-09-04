package sessions

import (
	"testing"
	"time"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestResolveForkBoundary(t *testing.T) {
	msgs := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("one")),
		chat.NewAssistantMessage(chat.NewTextPart("two")),
		chat.NewUserMessage(chat.NewTextPart("three")),
	}
	runs := []run.Run{
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_1", State: run.Completed, CreatedAt: time.Unix(1, 0), MessageMark: 2}),
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_2", State: run.Completed, CreatedAt: time.Unix(3, 0), MessageMark: 3}),
	}

	got, err := ResolveForkBoundary(msgs, runs, "run_1")
	if err != nil {
		t.Fatalf("resolve fork boundary: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("prefix len = %d, want 2", len(got.Messages))
	}
	// The prefix and the state the child inherits must name the same run, or a branch
	// gets a Plan its conversation never produced.
	if got.RunID != "run_1" {
		t.Fatalf("boundary run = %q, want run_1", got.RunID)
	}
	if len(got.RunIDs) != 1 || got.RunIDs[0] != "run_1" {
		t.Fatalf("boundary run projection = %v, want [run_1]", got.RunIDs)
	}
	msgs[0].Parts[0].Text = "source mutation"
	if text := got.Messages[0].Text(); text != "one" {
		t.Fatalf("boundary message after source mutation = %q, want owned snapshot", text)
	}
	got.Messages[1].Parts[0].Text = "boundary mutation"
	if text := msgs[1].Text(); text != "two" {
		t.Fatalf("source message after boundary mutation = %q, want unchanged", text)
	}
}

func TestResolveForkBoundaryExcludesActiveTail(t *testing.T) {
	msgs := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("complete")),
		chat.NewAssistantMessage(chat.NewTextPart("boundary")),
		chat.NewUserMessage(chat.NewTextPart("active")),
	}
	runs := []run.Run{
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_1", State: run.Completed, CreatedAt: time.Unix(1, 0), MessageMark: 2}),
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_2", State: run.Running, CreatedAt: time.Unix(2, 0), MessageMark: -1}),
		testsupport.MustRestoreRun(run.Snapshot{
			ID: "run_2_child", Lineage: run.Lineage{SpawnedByItemID: "item_task", ParentRunID: "run_2", RootRunID: "run_2"},
			State: run.Completed, CreatedAt: time.Unix(3, 0), MessageMark: 3,
		}),
	}

	got, err := ResolveForkBoundary(msgs, runs, "")
	if err != nil {
		t.Fatalf("resolve fork boundary: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("prefix len = %d, want terminal boundary 2", len(got.Messages))
	}
	if got.RunID != "run_1" {
		t.Fatalf("boundary run = %q, want the last terminal run run_1", got.RunID)
	}
	if len(got.RunIDs) != 1 || got.RunIDs[0] != "run_1" {
		t.Fatalf("boundary run projection = %v, want only the stable run", got.RunIDs)
	}
}

func TestResolveForkBoundaryRejectsActiveTarget(t *testing.T) {
	runs := []run.Run{testsupport.MustRestoreRun(run.Snapshot{ID: "run_active", State: run.Running, CreatedAt: time.Unix(1, 0), MessageMark: -1})}
	if _, err := ResolveForkBoundary([]chat.Message{chat.NewUserMessage(chat.NewTextPart("active"))}, runs, "run_active"); err != transcript.ErrRunNotFound {
		t.Fatalf("resolve active target error = %v, want ErrRunNotFound", err)
	}
}
