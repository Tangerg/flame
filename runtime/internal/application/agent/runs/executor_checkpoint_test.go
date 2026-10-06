package runs

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

const testExecutorBuildID = testsupport.BuildID

func testExecutorCheckpoint() ExecutorCheckpoint {
	return ExecutorCheckpoint{
		RootMemberID: "member_root",
		SessionID:    "ses_1",
		Payload:      []byte(`{"root":"member_root"}`),
		BuildID:      testExecutorBuildID,
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
	for _, test := range []struct{ name, root, session string }{
		{name: "root", root: "other_root", session: "ses_1"},
		{name: "session", root: "member_root", session: "other_session"},
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
			if err := barrier.validateFor(test.root, test.session); !errors.Is(err, ErrInvalidExecutorCheckpoint) {
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
	if err := barrier.validateFor("member_root", "ses_1"); err != nil {
		t.Fatalf("owned barrier no longer validates: %v", err)
	}
}
