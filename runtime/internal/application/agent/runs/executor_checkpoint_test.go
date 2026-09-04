package runs

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

const testExecutorBuildID = testsupport.BuildID

func mustCheckpointSelection(provider, model string) modelref.Selection {
	selection, err := modelref.New(provider, model)
	if err != nil {
		panic(err)
	}
	return selection
}

func testExecutorCheckpoint() ExecutorCheckpoint {
	return ExecutorCheckpoint{
		RootMemberID:   "member_root",
		Payload:        []byte(`{"root":"member_root"}`),
		BuildID:        testExecutorBuildID,
		Scope:          ExecutionScope{SessionID: "ses_1"},
		ModelSelection: mustCheckpointSelection("openai", "model"),
	}
}

func mustTreeInterrupted(
	t testing.TB,
	checkpoint ExecutorCheckpoint,
	interruptions []MemberInterruption,
) TreeInterrupted {
	t.Helper()
	barrier, err := NewTreeInterrupted(checkpoint, interruptions)
	if err != nil {
		t.Fatalf("NewTreeInterrupted: %v", err)
	}
	return barrier
}

func TestTreeInterruptedRejectsCheckpointBoundToDifferentApplicationFacts(t *testing.T) {
	for _, test := range []struct {
		name              string
		root              string
		session           string
		goalIncarnationID string
		selection         modelref.Selection
	}{
		{name: "root", root: "other_root", session: "ses_1", selection: mustCheckpointSelection("openai", "model")},
		{name: "session", root: "member_root", session: "other_session", selection: mustCheckpointSelection("openai", "model")},
		{name: "goal incarnation", root: "member_root", session: "ses_1", goalIncarnationID: "other_goal", selection: mustCheckpointSelection("openai", "model")},
		{name: "provider", root: "member_root", session: "ses_1", selection: mustCheckpointSelection("anthropic", "model")},
		{name: "model", root: "member_root", session: "ses_1", selection: mustCheckpointSelection("openai", "gpt-other")},
	} {
		t.Run(test.name, func(t *testing.T) {
			barrier := mustTreeInterrupted(t, testExecutorCheckpoint(), []MemberInterruption{{
				MemberID:  "member_root",
				RequestID: "request_root",
				Interrupt: Interrupt{
					Kind: interrupt.Question,
					Question: &QuestionPrompt{
						ToolName:  "ask_user",
						Arguments: `{}`,
						Fields:    []QuestionFieldSpec{{Prompt: "Continue?", Header: "Continue"}},
					},
				},
			}})
			if err := barrier.validateFor(test.root, test.session, test.goalIncarnationID, test.selection); !errors.Is(err, ErrInvalidExecutorCheckpoint) {
				t.Fatalf("validateFor error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
		})
	}
}

func TestTreeInterruptedOwnsCheckpointAndInterruptions(t *testing.T) {
	checkpoint := testExecutorCheckpoint()
	interruptions := []MemberInterruption{{
		MemberID:  "member_root",
		RequestID: "request_root",
		Interrupt: Interrupt{
			Kind: interrupt.Question,
			Question: &QuestionPrompt{
				ToolName: "ask_user", Arguments: `{}`,
				Fields: []QuestionFieldSpec{{
					Prompt: "Continue?", Header: "Continue",
					Options: []QuestionOptionSpec{
						{Label: "Yes", Description: "Proceed"},
						{Label: "No", Description: "Stop"},
					},
				}},
			},
		},
	}}
	barrier := mustTreeInterrupted(t, checkpoint, interruptions)

	checkpoint.Payload[0] = 'x'
	interruptions[0].MemberID = "member_changed"
	interruptions[0].Interrupt.Question.Fields[0].Options[0].Label = "Changed"
	projectedCheckpoint := barrier.Checkpoint()
	projectedCheckpoint.Payload[0] = 'y'
	projectedInterruptions := barrier.Interruptions()
	projectedInterruptions[0].MemberID = "member_projected"
	projectedInterruptions[0].Interrupt.Question.Fields[0].Options[0].Label = "Projected"

	ownedCheckpoint := barrier.Checkpoint()
	ownedInterruptions := barrier.Interruptions()
	if string(ownedCheckpoint.Payload) != `{"root":"member_root"}` {
		t.Fatalf("owned checkpoint payload = %q", ownedCheckpoint.Payload)
	}
	if ownedInterruptions[0].MemberID != "member_root" ||
		ownedInterruptions[0].Interrupt.Question.Fields[0].Options[0].Label != "Yes" {
		t.Fatalf("owned interruptions = %+v", ownedInterruptions)
	}
	if err := barrier.validateFor(
		"member_root", "ses_1", "", mustCheckpointSelection("openai", "model"),
	); err != nil {
		t.Fatalf("owned barrier no longer validates: %v", err)
	}
}
