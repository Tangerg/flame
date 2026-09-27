package conversation

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestStartRunValidation(t *testing.T) {
	valid := prompt.StartRun{
		SessionID: "ses_1", Message: prompt.Message{Text: "hello"},
		Options: prompt.RunOptions{Provider: "mock", Model: "balanced"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Options.Model = ""
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "provider and model must be set together") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeleteSessionValidatesItsOptionalMutationIdentity(t *testing.T) {
	if err := (DeleteSession{SessionID: "ses_1"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (DeleteSession{CommandID: replay.CommandID("invalid"), SessionID: "ses_1"}).Validate(); err == nil {
		t.Fatal("invalid deletion command identity was accepted")
	}
	if err := (DeleteSession{}).Validate(); err == nil {
		t.Fatal("empty deletion target was accepted")
	}
}

func TestStartRunEqualUsesTheCompleteMutationFingerprint(t *testing.T) {
	request := prompt.StartRun{
		CommandID: replay.CommandID("cli_11111111111111111111111111111111"), SessionID: "ses_1",
		Message: prompt.Message{Text: "hello"}, Options: prompt.RunOptions{Provider: "mock", Model: "balanced"},
	}
	if !request.Equal(request.Clone()) {
		t.Fatal("cloned start request is not equal")
	}
	changed := request.Clone()
	changed.Message.Text = "different"
	if request.Equal(changed) {
		t.Fatal("different start payloads are equal")
	}
}

func TestResumeRunRequiresCompleteUniqueSet(t *testing.T) {
	request := ResumeRun{RunID: "run_1", Answers: []InterruptAnswer{
		{ItemID: "a", Answer: ApprovalAnswer{Decision: protocol.ApprovalApprove}},
		{ItemID: "q", Answer: QuestionAnswer{Values: [][]string{{"yes"}}}},
	}}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.Answers[1].ItemID = "a"
	if err := request.Validate(); err == nil {
		t.Fatal("duplicate interrupt was accepted")
	}
	request.Answers = []InterruptAnswer{{ItemID: " a", Answer: ApprovalAnswer{Decision: protocol.ApprovalApprove}}}
	if err := request.Validate(); err == nil {
		t.Fatal("resume accepted an item identity that requires trimming")
	}
}

func TestSubscribeRunNeedsRunAndSegment(t *testing.T) {
	if err := (SubscribeRun{RunID: "run_1", SegmentID: "seg_1", AfterEventID: "opaque"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (SubscribeRun{RunID: "run_1"}).Validate(); err == nil {
		t.Fatal("missing segment was accepted")
	}
	if err := (SubscribeRun{RunID: " run_1", SegmentID: "seg_1"}).Validate(); err == nil {
		t.Fatal("subscription accepted a run identity that requires trimming")
	}
	if err := (SubscribeRun{RunID: "run_1", SegmentID: "seg_1 "}).Validate(); err == nil {
		t.Fatal("subscription accepted a segment identity that requires trimming")
	}
}

func TestSnapshotSubscriptionRequiresItsSessionWithoutAReplayCursor(t *testing.T) {
	request := SubscribeRun{RunID: "run_1", SegmentID: "seg_1", Snapshot: true}
	if err := request.Validate(); err == nil {
		t.Fatal("snapshot subscription accepted no Session identity")
	}
	request.SessionID = "ses_1"
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.AfterEventID = "evt_opaque"
	if err := request.Validate(); err == nil {
		t.Fatal("snapshot subscription accepted a replay cursor")
	}
}

func TestCancelRunUsesRuntimeWireConstraints(t *testing.T) {
	if err := (CancelRun{RunID: "run_1", Reason: strings.Repeat("界", 1025)}).Validate(); err == nil {
		t.Fatal("oversized cancellation reason was accepted")
	}
}

func TestSegmentStreamValidatesOperationSpecificUserItemIdentity(t *testing.T) {
	stream := SegmentStream{RunID: "run_1", SegmentID: "seg_1", Events: func(func(RunEvent, error) bool) {}}
	if err := stream.ValidateStart(); err == nil {
		t.Fatal("start stream without a user item id was accepted")
	}
	stream.UserItemID = "item_1"
	if err := stream.ValidateStart(); err != nil {
		t.Fatal(err)
	}
	if err := stream.ValidateSubscription(); err == nil {
		t.Fatal("subscription stream with a user item id was accepted")
	}
	if err := stream.ValidateResume("run_1", &prompt.Message{Text: "continue"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.ValidateResume("run_1", nil); err == nil {
		t.Fatal("response-only resume with a user item id was accepted")
	}
	stream.UserItemID = ""
	if err := stream.ValidateSubscription(); err != nil {
		t.Fatal(err)
	}
	if err := stream.ValidateResume("run_1", nil); err != nil {
		t.Fatal(err)
	}
	if err := stream.ValidateResume("run_other", nil); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched resume target error = %v", err)
	}
	stream.UserItemID = " item_1"
	if err := stream.ValidateStart(); err == nil {
		t.Fatal("start stream accepted a user item identity that requires trimming")
	}
	stream.UserItemID = "item_1"
	stream.HeadEventID = " event_1"
	if err := stream.ValidateStart(); err == nil {
		t.Fatal("start stream accepted a head event identity that requires trimming")
	}
}
