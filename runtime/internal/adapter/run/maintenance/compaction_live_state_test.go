package maintenance

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestLiveStateReminderRendersRetainedShells(t *testing.T) {
	msg, ok := liveStateReminder(LiveStateSnapshot{
		Shells: []RetainedShell{{ID: "bg_1", Command: "npm run dev"}},
	})
	if !ok {
		t.Fatal("non-empty snapshot should render a reminder")
	}
	body := msg.Text()
	for _, want := range []string{"<system-reminder>", "bg_1", "npm run dev", "read_shell_output"} {
		if !strings.Contains(body, want) {
			t.Fatalf("reminder missing %q:\n%s", want, body)
		}
	}
}

func TestLiveStateReminderEmptyIsSkipped(t *testing.T) {
	if _, ok := liveStateReminder(LiveStateSnapshot{}); ok {
		t.Fatal("empty snapshot must not render a reminder")
	}
}

// The full summary rung must preserve handles even after their processes exit.
func TestCompactorPreservesUnreadCompletedShell(t *testing.T) {
	store := newCompactionTestStore()
	const sessID = "sess-live"
	const total = 20
	for range total {
		_ = store.Write(context.Background(), sessID, chat.NewUserMessage(chat.NewTextPart("msg")))
	}
	client, _ := chatclient.New(newTextStubModel("BULLETS"), chatclient.Config{})

	shells := exec.NewShells(nil, false)
	t.Cleanup(func() { _ = shells.KillAll() })
	id, err := shells.Launch(t.Context(), sessID, "", "printf unread-result", exec.Timeout{}, false)
	if err != nil {
		t.Fatal(err)
	}
	sh, ok := shells.Get(id)
	if !ok {
		t.Fatal("launched shell is missing")
	}
	<-sh.Done()
	live := NewLiveStateSnapshotter(shells)

	history, _ := store.Read(context.Background(), sessID)
	threshold := mustEstimateModelContextTokens(t, history, nil, chat.Options{}) - 1
	c := mustNewCompactor(t, store, constClient(client), live)
	res, err := c.compactModelContext(context.Background(), durableContextRequest(t, sessID, history, 0, nil), testInputLimits(t, threshold))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Summarized() {
		t.Fatal("expected compaction to fire")
	}

	after, _ := store.Read(context.Background(), sessID)
	// [summary, reminder, latest turn]
	if len(after) != 3 {
		t.Fatalf("post-compact len = %d, want 3 (summary + reminder + latest turn)", len(after))
	}
	if !strings.HasPrefix(after[0].Text(), "[Earlier conversation summary]") {
		t.Fatalf("after[0] should be the summary, got %q", after[0].Text())
	}
	reminder := after[1].Text()
	if !strings.Contains(reminder, "<system-reminder>") || !strings.Contains(reminder, id) {
		t.Fatalf("after[1] should be the live-state reminder, got %q", reminder)
	}
	output, dropped := sh.Read()
	if output != "unread-result" || dropped {
		t.Fatalf("post-compaction shell output = %q, dropped=%t", output, dropped)
	}
	shells.Remove(id)
	if !live(t.Context(), sessID).empty() {
		t.Fatal("released shell remained in the compaction snapshot")
	}
}

// TestCompactorSkipsReminderWhenNoLiveState confirms an empty snapshot leaves the
// rewritten history exactly [summary, ...recent] — no stray reminder message.
func TestCompactorSkipsReminderWhenNoLiveState(t *testing.T) {
	store := newCompactionTestStore()
	const sessID = "sess-live-empty"
	const total = 20
	for range total {
		_ = store.Write(context.Background(), sessID, chat.NewUserMessage(chat.NewTextPart("msg")))
	}
	client, _ := chatclient.New(newTextStubModel("BULLETS"), chatclient.Config{})

	live := func(context.Context, string) LiveStateSnapshot { return LiveStateSnapshot{} }
	history, _ := store.Read(context.Background(), sessID)
	threshold := mustEstimateModelContextTokens(t, history, nil, chat.Options{}) - 1
	c := mustNewCompactor(t, store, constClient(client), live)
	if _, err := c.compactModelContext(context.Background(), durableContextRequest(t, sessID, history, 0, nil), testInputLimits(t, threshold)); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Read(context.Background(), sessID)
	if len(after) != 2 {
		t.Fatalf("empty live-state should leave summary + latest turn = 2, got %d", len(after))
	}
	for _, m := range after {
		if strings.Contains(m.Text(), "<system-reminder>") {
			t.Fatalf("no reminder should be injected for an empty snapshot: %q", m.Text())
		}
	}
}
