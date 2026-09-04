package runs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

type recordingCompactions struct {
	runs []run.Run
	plan ConversationCompactionPlan
}

type mutatingConversationStore struct {
	*testsupport.ConversationStore
	onRead  func()
	onCount func()
}

func (m *mutatingConversationStore) Read(ctx context.Context, sessionID string) ([]chat.Message, error) {
	if m.onRead != nil {
		m.onRead()
	}
	return m.ConversationStore.Read(ctx, sessionID)
}

func (m *mutatingConversationStore) Count(ctx context.Context, sessionID string) (int, error) {
	if m.onCount != nil {
		m.onCount()
	}
	return m.ConversationStore.Count(ctx, sessionID)
}

func (r *recordingCompactions) ListRuns(context.Context, string) ([]run.Run, error) {
	return append([]run.Run(nil), r.runs...), nil
}

func (r *recordingCompactions) ApplyCompaction(_ context.Context, plan ConversationCompactionPlan) error {
	r.plan = plan
	return nil
}

func TestMessagesCoordinatesDurableHistory(t *testing.T) {
	messages := NewConversationHistory(testsupport.NewConversationStore(), nil)
	seed := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("one")),
		chat.NewAssistantMessage(chat.NewTextPart("two")),
		chat.NewUserMessage(chat.NewTextPart("three")),
	}
	if err := messages.Seed(t.Context(), "ses_1", seed); err != nil {
		t.Fatal(err)
	}
	if err := messages.Seed(t.Context(), "ses_1", seed); !errors.Is(err, conversation.ErrNotEmpty) {
		t.Fatalf("second seed error = %v", err)
	}
	if err := messages.Truncate(t.Context(), "ses_1", 2); err != nil {
		t.Fatal(err)
	}
	if err := messages.Append(t.Context(), "ses_1", chat.NewUserMessage(chat.NewTextPart("four"))); err != nil {
		t.Fatal(err)
	}
	got, err := messages.Read(t.Context(), "ses_1")
	if err != nil || len(got) != 3 || got[1].Text() != "two" || got[2].Text() != "four" {
		t.Fatalf("Read = %#v, %v", got, err)
	}
}

func TestMessagesOwnWriteInputsBeforePersistenceReads(t *testing.T) {
	seed := []chat.Message{chat.NewUserMessage(chat.NewTextPart("seed"))}
	store := &mutatingConversationStore{
		ConversationStore: testsupport.NewConversationStore(),
		onCount:           func() { seed[0].Parts[0].Text = "changed seed" },
	}
	history := NewConversationHistory(store, nil)
	if err := history.Seed(t.Context(), "ses_1", seed); err != nil {
		t.Fatal(err)
	}

	addition := []chat.Message{chat.NewAssistantMessage(chat.NewTextPart("addition"))}
	store.onRead = func() { addition[0].Parts[0].Text = "changed addition" }
	if err := history.Append(t.Context(), "ses_1", addition...); err != nil {
		t.Fatal(err)
	}

	stored, err := store.ConversationStore.Read(t.Context(), "ses_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 || stored[0].Text() != "seed" || stored[1].Text() != "addition" {
		t.Fatalf("conversation after persistence callbacks changed caller input = %#v", stored)
	}
}

func TestMessagesRejectsMissingSession(t *testing.T) {
	messages := NewConversationHistory(testsupport.NewConversationStore(), nil)
	for _, sessionID := range []string{
		"",
		" ses_1",
		"ses_ one",
		"ses_\u200bhidden",
		strings.Repeat("界", runtimeidentity.MaximumResourceCharacters+1),
	} {
		if _, err := messages.Read(t.Context(), sessionID); !errors.Is(err, errConversationSessionIDRequired) {
			t.Errorf("Read(%q) error = %v", sessionID, err)
		}
		if err := messages.Append(t.Context(), sessionID, chat.NewUserMessage(chat.NewTextPart("one"))); !errors.Is(err, errConversationSessionIDRequired) {
			t.Errorf("Append(%q) error = %v", sessionID, err)
		}
	}
}

func TestMessagesPlansCompactionRunWatermarks(t *testing.T) {
	at := time.Unix(10, 0).UTC()
	compactions := &recordingCompactions{runs: []run.Run{
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_before", SessionID: "ses_1", State: run.Completed, CreatedAt: at, MessageMark: 4}),
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_cut", SessionID: "ses_1", State: run.Completed, CreatedAt: at.Add(time.Second), MessageMark: 6}),
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_recent", SessionID: "ses_1", State: run.Completed, CreatedAt: at.Add(2 * time.Second), MessageMark: 8}),
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_active", SessionID: "ses_1", State: run.Running, CreatedAt: at.Add(3 * time.Second)}),
	}}
	messages := NewConversationHistory(testsupport.NewConversationStore(), compactions)
	replacement := []chat.Message{
		chat.NewSystemMessage("summary"),
		chat.NewUserMessage(chat.NewTextPart("recent question")),
		chat.NewAssistantMessage(chat.NewTextPart("recent answer")),
	}
	if err := messages.RewriteForCompaction(t.Context(), "ses_1", 8, 6, 1, replacement...); err != nil {
		t.Fatal(err)
	}
	planned := compactions.plan.Runs()
	if len(planned) != 4 {
		t.Fatalf("planned Runs = %d, want 4", len(planned))
	}
	wantMarks := []int{1, 1, 3, run.UnknownMessageMark}
	for index, replacement := range planned {
		if !replacement.Expected().Equal(compactions.runs[index]) {
			t.Fatalf("planned Run %d lost its expected CAS aggregate", index)
		}
		if got := replacement.State().MessageMark(); got != wantMarks[index] {
			t.Errorf("replacement mark[%d] = %d, want %d", index, got, wantMarks[index])
		}
	}
}
