package agentexec

import (
	"encoding/base64"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	agent "github.com/Tangerg/scope/agent"
)

func TestInteractionPendingSteersRoundTripCanonicalContent(t *testing.T) {
	firstID, err := agent.ParseSignalID("steer:02")
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := agent.ParseSignalID("steer:01")
	if err != nil {
		t.Fatal(err)
	}
	pending := map[agent.SignalID]pendingInteractionSteer{
		firstID: {content: []transcript.ContentBlock{{
			Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte{0, 1, 2},
		}}},
		secondID: {content: []transcript.ContentBlock{{
			Kind: transcript.TextContent, Text: "revise",
		}}},
	}
	wire, err := encodeInteractionPendingSteers(pending)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 2 || wire[0].SignalID != secondID.String() || wire[1].SignalID != firstID.String() {
		t.Fatalf("pending steer order = %#v", wire)
	}
	decoded, err := decodeInteractionPendingSteers(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, pending) {
		t.Fatalf("decoded pending steers = %#v, want %#v", decoded, pending)
	}
}

func TestInteractionPendingContinuationRoundTripsCanonicalContent(t *testing.T) {
	rootID, err := agent.ParseProcessID("process:root")
	if err != nil {
		t.Fatal(err)
	}
	pending := &pendingInteractionContinuation{
		processID: rootID,
		itemID:    "item_followup",
		content: []transcript.ContentBlock{{
			Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte{0, 1, 2},
		}},
	}
	wire, err := encodeInteractionPendingContinuation(pending, rootID)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeInteractionPendingContinuation(wire, rootID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, pending) {
		t.Fatalf("decoded pending continuation = %#v, want %#v", decoded, pending)
	}
	foreignID, err := agent.ParseProcessID("process:foreign")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeInteractionPendingContinuation(wire, foreignID); err == nil {
		t.Fatal("decode accepted a continuation for a foreign root")
	}
}

func TestDecodeInteractionPendingSteersRejectsNoncanonicalWire(t *testing.T) {
	validImage := base64.StdEncoding.EncodeToString([]byte{1})
	tests := map[string][]interactionPendingSteerWire{
		"unordered": {
			{SignalID: "steer:02", Content: []interactionContentBlockWire{{Kind: "text", Text: "two"}}},
			{SignalID: "steer:01", Content: []interactionContentBlockWire{{Kind: "text", Text: "one"}}},
		},
		"duplicate": {
			{SignalID: "steer:01", Content: []interactionContentBlockWire{{Kind: "text", Text: "one"}}},
			{SignalID: "steer:01", Content: []interactionContentBlockWire{{Kind: "text", Text: "again"}}},
		},
		"empty content": {{SignalID: "steer:01"}},
		"mixed text": {{
			SignalID: "steer:01",
			Content:  []interactionContentBlockWire{{Kind: "text", Text: "one", MediaType: "text/plain"}},
		}},
		"mixed image": {{
			SignalID: "steer:01",
			Content: []interactionContentBlockWire{{
				Kind: "image", Text: "caption", MediaType: "image/png", Data: validImage,
			}},
		}},
		"non-image media": {{
			SignalID: "steer:01",
			Content:  []interactionContentBlockWire{{Kind: "image", MediaType: "text/plain", Data: validImage}},
		}},
		"invalid base64": {{
			SignalID: "steer:01",
			Content:  []interactionContentBlockWire{{Kind: "image", MediaType: "image/png", Data: "*"}},
		}},
		"noncanonical base64": {{
			SignalID: "steer:01",
			Content:  []interactionContentBlockWire{{Kind: "image", MediaType: "image/png", Data: "A\nQ=="}},
		}},
		"unknown kind": {{
			SignalID: "steer:01",
			Content:  []interactionContentBlockWire{{Kind: "audio", Data: validImage}},
		}},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeInteractionPendingSteers(values); err == nil {
				t.Fatal("decode succeeded")
			}
		})
	}
}

func TestDecodeInteractionCheckpointRejectsUnknownFields(t *testing.T) {
	if _, err := decodeInteractionCheckpointPayload([]byte(`{"unexpected":true}`)); err == nil {
		t.Fatal("decode accepted an unknown checkpoint field")
	}
}

func TestDecodeInteractionCheckpointRejectsDuplicateJSONMembers(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "root",
			payload: `{"tree":{},"tree":{}}`,
			want:    `duplicate JSON member "tree" at $`,
		},
		{
			name:    "nested tree",
			payload: `{"tree":{"state":"first","state":"second"}}`,
			want:    `duplicate JSON member "state" at $.tree`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodeInteractionCheckpointPayload([]byte(test.payload))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("decode error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestInteractionModelContextsRoundTripCanonicalCalibration(t *testing.T) {
	first, err := agent.ParseProcessID("process:context-02")
	if err != nil {
		t.Fatal(err)
	}
	second, err := agent.ParseProcessID("process:context-01")
	if err != nil {
		t.Fatal(err)
	}
	firstCalibration, err := NewModelContextTokenCalibration(12_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	secondCalibration, err := NewModelContextTokenCalibration(8_000, 9_000)
	if err != nil {
		t.Fatal(err)
	}
	contexts := map[agent.ProcessID]ModelContextTokenCalibration{
		first: firstCalibration, second: secondCalibration,
	}

	accounted := map[agent.ProcessID]struct{}{first: {}, second: {}}
	wire, err := encodeInteractionModelContexts(contexts, accounted)
	if err != nil {
		t.Fatal(err)
	}
	if len(wire) != 2 || wire[0].MemberID != second.String() || wire[1].MemberID != first.String() {
		t.Fatalf("model context order = %#v", wire)
	}
	// A calibration is only carried for a member this checkpoint accounts calls
	// for, so dropping that member's accounting drops its calibration with it.
	unaccounted, err := encodeInteractionModelContexts(contexts, map[agent.ProcessID]struct{}{first: {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(unaccounted) != 1 || unaccounted[0].MemberID != first.String() {
		t.Fatalf("unaccounted model contexts = %#v", unaccounted)
	}
	processes := map[agent.ProcessID]struct{}{first: {}, second: {}}
	calls := map[agent.ProcessID]map[string]int{
		first: {"model": 1}, second: {"model": 1},
	}
	decoded, err := decodeInteractionModelContexts(wire, processes, calls)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, contexts) {
		t.Fatalf("decoded model contexts = %#v, want %#v", decoded, contexts)
	}
}

func TestModelContextTokenCalibrationKeepsExactMaxIntDelta(t *testing.T) {
	calibration, err := NewModelContextTokenCalibration(int64(math.MaxInt), math.MaxInt)
	if err != nil {
		t.Fatal(err)
	}
	if adjustment := calibration.Adjustment(); adjustment != 0 {
		t.Fatalf("exact MaxInt calibration adjustment = %d, want 0", adjustment)
	}
}

// TestAdmitCheckpointMemberOwnsBothMemberListRules tests the rule where it
// lives. Through either decoder a rejected row is also rejected by the check
// after it, so neither list can show which one refused.
func TestAdmitCheckpointMemberOwnsBothMemberListRules(t *testing.T) {
	first, err := agent.ParseProcessID("process:a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := agent.ParseProcessID("process:b")
	if err != nil {
		t.Fatal(err)
	}
	known := map[agent.ProcessID]struct{}{first: {}, second: {}}

	if _, err := admitCheckpointMember(first.String(), "", 0, known); err != nil {
		t.Fatalf("the first row of a known member list was refused: %v", err)
	}
	if _, err := admitCheckpointMember(second.String(), first.String(), 1, known); err != nil {
		t.Fatalf("an ascending known member was refused: %v", err)
	}
	if _, err := admitCheckpointMember(first.String(), second.String(), 1, known); err == nil {
		t.Fatal("a member out of canonical order was admitted")
	}
	if _, err := admitCheckpointMember(first.String(), first.String(), 1, known); err == nil {
		t.Fatal("a repeated member was admitted")
	}
	if _, err := admitCheckpointMember(second.String(), "", 0, map[agent.ProcessID]struct{}{first: {}}); err == nil {
		t.Fatal("a member outside the checkpoint was admitted")
	}
}

func TestDecodeInteractionModelContextsRejectsForeignOrUnaccountedMember(t *testing.T) {
	processID, err := agent.ParseProcessID("process:context")
	if err != nil {
		t.Fatal(err)
	}
	wire := []interactionModelContextWire{{
		MemberID: processID.String(), ReportedTokens: 100, EstimatedTokens: 90,
	}}
	if _, err := decodeInteractionModelContexts(wire, nil, nil); err == nil {
		t.Fatal("foreign model context decoded")
	}
	if _, err := decodeInteractionModelContexts(
		wire,
		map[agent.ProcessID]struct{}{processID: {}},
		nil,
	); err == nil {
		t.Fatal("unaccounted model context decoded")
	}
}

// TestCheckpointDecodeErrorsNameTheirSectionExactlyOnce pins where the section
// context lives. decodeInteractionCheckpointPayload names the checkpoint
// section that failed; a section decoder reports only the defect. A decoder
// that spelled the owner itself would double it in the message an operator
// reads.
func TestCheckpointDecodeErrorsNameTheirSectionExactlyOnce(t *testing.T) {
	for _, section := range []struct {
		name string
		call func() error
	}{
		{name: "members", call: func() error {
			_, err := decodeInteractionCheckpointMembers(
				[]interactionMemberCallsWire{{MemberID: "b"}, {MemberID: "a"}}, nil)
			return err
		}},
		{name: "model contexts", call: func() error {
			_, err := decodeInteractionModelContexts(
				[]interactionModelContextWire{{MemberID: "b"}, {MemberID: "a"}}, nil, nil)
			return err
		}},
		{name: "carried calls", call: func() error {
			_, err := decodeInteractionCallCounts(
				[]interactionModelCallsWire{{Model: "b", Calls: 1}, {Model: "a", Calls: 1}})
			return err
		}},
	} {
		t.Run(section.name, func(t *testing.T) {
			err := section.call()
			if err == nil {
				t.Fatal("malformed section decoded")
			}
			if strings.Contains(err.Error(), "agentexec:") {
				t.Errorf("section decoder names its own owner: %v", err)
			}
		})
	}

	_, err := decodeInteractionCheckpointPayload([]byte(`{"tree":{}}`))
	if err == nil {
		t.Fatal("empty checkpoint tree decoded")
	}
	if count := strings.Count(err.Error(), "agentexec: Interaction checkpoint"); count != 1 {
		t.Fatalf("payload error names its section %d times: %v", count, err)
	}
}

// TestToolDecisionsNameOneOutcome pins what the removed combination validators
// used to check at run time: a denial carries a reason and nothing else, and an
// allowed call carries no reason. The pairs those checks rejected can no longer
// be written, so only the wording is left to prove.
func TestToolDecisionsNameOneOutcome(t *testing.T) {
	for _, blank := range []string{"", "   ", "\n\t"} {
		reason, denied := DenyTool(blank).Denied()
		if !denied || reason != "denied by Tool policy" {
			t.Fatalf("DenyTool(%q) = (%q, %v)", blank, reason, denied)
		}
		hookReason, hookDenied := DenyToolHook(blank).Denied()
		if !hookDenied || hookReason != "denied by a PreToolUse hook" {
			t.Fatalf("DenyToolHook(%q) = (%q, %v)", blank, hookReason, hookDenied)
		}
	}
	stated := DenyTool("  the gate refused it  ")
	if reason, denied := stated.Denied(); !denied || reason != "the gate refused it" {
		t.Fatalf("stated denial = (%q, %v)", reason, denied)
	}
	if _, ok := stated.EffectiveArguments(); ok {
		t.Fatal("a denial carries replacement arguments")
	}
	if _, ok := stated.Approval(); ok {
		t.Fatal("a denial waits on an approval")
	}
	if _, denied := AllowTool().Denied(); denied {
		t.Fatal("AllowTool denies the call")
	}
	if _, denied := AllowToolHook(true, nil).Denied(); denied {
		t.Fatal("an escalating hook denies the call")
	}
	if !AllowToolHook(true, nil).RequiresApproval() {
		t.Fatal("an escalating hook does not require approval")
	}
}
