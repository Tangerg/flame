package runs

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestConversationCompactionPlanOwnsOneSessionRunSet(t *testing.T) {
	compaction, err := conversation.NewCompaction(1, 0, 0, []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("trimmed")),
	})
	if err != nil {
		t.Fatal(err)
	}
	current := testsupport.MustRestoreRun(run.Snapshot{
		ID: "run_1", SessionID: "ses_1", State: run.Completed,
		CreatedAt: time.Unix(1, 0).UTC(), MessageMark: 1,
	})
	replacement := testsupport.MustRunReplacement(current, current)
	input := []run.Replacement{replacement}
	plan, err := NewConversationCompactionPlan("ses_1", compaction, input)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = run.Replacement{}
	got := plan.Runs()
	if plan.SessionID() != "ses_1" || plan.Compaction().ExpectedCount() != 1 ||
		len(got) != 1 || !got[0].Expected().Equal(current) {
		t.Fatalf("plan lost its exact write-set: session=%q runs=%+v", plan.SessionID(), got)
	}
	got[0] = run.Replacement{}
	if err := plan.Validate(); err != nil {
		t.Fatalf("returned Run slice mutated plan: %v", err)
	}
}

func TestConversationCompactionPlanRejectsInvalidSessionRunSets(t *testing.T) {
	compaction, err := conversation.NewCompaction(0, 0, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{"", " ses_1", strings.Repeat("界", runtimeidentity.MaximumResourceCharacters+1)} {
		if _, err := NewConversationCompactionPlan(sessionID, compaction, nil); err == nil {
			t.Errorf("NewConversationCompactionPlan(%q) accepted an invalid Session", sessionID)
		}
	}
	foreign := testsupport.MustRestoreRun(run.Snapshot{
		ID: "run_foreign", SessionID: "ses_other", State: run.Completed,
		CreatedAt: time.Unix(1, 0).UTC(), MessageMark: 0,
	})
	replacement := testsupport.MustRunReplacement(foreign, foreign)
	if _, err := NewConversationCompactionPlan("ses_1", compaction, []run.Replacement{replacement}); err == nil {
		t.Fatal("NewConversationCompactionPlan accepted a foreign-Session Run")
	}
	if _, err := NewConversationCompactionPlan("ses_other", compaction, []run.Replacement{replacement, replacement}); err == nil {
		t.Fatal("NewConversationCompactionPlan accepted a repeated Run")
	}
	if err := (ConversationCompactionPlan{}).Validate(); err == nil {
		t.Fatal("zero ConversationCompactionPlan is valid")
	}
}
