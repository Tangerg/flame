package execution

import (
	"encoding/base64"
	"encoding/json/jsontext"
	"errors"
	"math"

	"testing"

	scoperun "github.com/Tangerg/scope/agent/run"
)

func TestDecodeInteractionPendingSteersRejectsNoncanonicalWire(t *testing.T) {
	validImage := base64.StdEncoding.EncodeToString([]byte{1})
	tests := map[string][]interactionPendingSteerWire{
		"missing reserved item": {{SignalID: "steer:01", Content: []interactionContentBlockWire{{Kind: "text", Text: "one"}}}},
		"unordered": {
			{ItemID: "item_steer", SignalID: "steer:02", Content: []interactionContentBlockWire{{Kind: "text", Text: "two"}}},
			{ItemID: "item_steer", SignalID: "steer:01", Content: []interactionContentBlockWire{{Kind: "text", Text: "one"}}},
		},
		"duplicate": {
			{ItemID: "item_steer", SignalID: "steer:01", Content: []interactionContentBlockWire{{Kind: "text", Text: "one"}}},
			{ItemID: "item_steer", SignalID: "steer:01", Content: []interactionContentBlockWire{{Kind: "text", Text: "again"}}},
		},
		"empty content": {{ItemID: "item_steer", SignalID: "steer:01"}},
		"mixed text": {{
			ItemID: "item_steer", SignalID: "steer:01",
			Content: []interactionContentBlockWire{{Kind: "text", Text: "one", MediaType: "text/plain"}},
		}},
		"mixed image": {{
			ItemID: "item_steer", SignalID: "steer:01",
			Content: []interactionContentBlockWire{{
				Kind: "image", Text: "caption", MediaType: "image/png", Data: validImage,
			}},
		}},
		"non-image media": {{
			ItemID: "item_steer", SignalID: "steer:01",
			Content: []interactionContentBlockWire{{Kind: "image", MediaType: "text/plain", Data: validImage}},
		}},
		"invalid base64": {{
			ItemID: "item_steer", SignalID: "steer:01",
			Content: []interactionContentBlockWire{{Kind: "image", MediaType: "image/png", Data: "*"}},
		}},
		"noncanonical base64": {{
			ItemID: "item_steer", SignalID: "steer:01",
			Content: []interactionContentBlockWire{{Kind: "image", MediaType: "image/png", Data: "A\nQ=="}},
		}},
		"unknown kind": {{
			ItemID: "item_steer", SignalID: "steer:01",
			Content: []interactionContentBlockWire{{Kind: "audio", Data: validImage}},
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
		pointer jsontext.Pointer
	}{
		{
			name:    "root",
			payload: `{"handoff":{},"handoff":{}}`,
			pointer: "/handoff",
		},
		{
			name:    "nested handoff",
			payload: `{"handoff":{"root_id":"first","root_id":"second"}}`,
			pointer: "/handoff/root_id",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodeInteractionCheckpointPayload([]byte(test.payload))
			syntax, ok := errors.AsType[*jsontext.SyntacticError](err)
			if !ok || !errors.Is(err, jsontext.ErrDuplicateName) || syntax.JSONPointer != test.pointer {
				t.Fatalf("decode error = %v, want duplicate member at %s", err, test.pointer)
			}
		})
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
	first, err := scoperun.ParseProcessID("process:a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := scoperun.ParseProcessID("process:b")
	if err != nil {
		t.Fatal(err)
	}
	known := map[scoperun.ProcessID]struct{}{first: {}, second: {}}

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
	if _, err := admitCheckpointMember(second.String(), "", 0, map[scoperun.ProcessID]struct{}{first: {}}); err == nil {
		t.Fatal("a member outside the checkpoint was admitted")
	}
}

func TestDecodeInteractionModelContextsRejectsForeignOrUnaccountedMember(t *testing.T) {
	processID, err := scoperun.ParseProcessID("process:context")
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
		map[scoperun.ProcessID]struct{}{processID: {}},
		nil,
	); err == nil {
		t.Fatal("unaccounted model context decoded")
	}
}

// TestToolDecisionsNameOneOutcome pins what the removed combination validators
// used to check at run time: a denial carries a reason and nothing else, and an
// allowed call carries no reason. The pairs those checks rejected can no longer
// be written, so only the wording is left to prove.
func TestToolDecisionsNameOneOutcome(t *testing.T) {
	for _, blank := range []string{"", "   ", "\n\t"} {
		reason, denied := denyTool(blank).Denied()
		if !denied || reason != "denied by Tool policy" {
			t.Fatalf("DenyTool(%q) = (%q, %v)", blank, reason, denied)
		}
		hookReason, hookDenied := denyToolHook(blank).Denied()
		if !hookDenied || hookReason != "denied by a PreToolUse hook" {
			t.Fatalf("DenyToolHook(%q) = (%q, %v)", blank, hookReason, hookDenied)
		}
	}
	stated := denyTool("  the gate refused it  ")
	if reason, denied := stated.Denied(); !denied || reason != "the gate refused it" {
		t.Fatalf("stated denial = (%q, %v)", reason, denied)
	}
	if _, ok := stated.EffectiveArguments(); ok {
		t.Fatal("a denial carries replacement arguments")
	}
	if _, ok := stated.Approval(); ok {
		t.Fatal("a denial waits on an approval")
	}
	if _, denied := allowTool().Denied(); denied {
		t.Fatal("AllowTool denies the call")
	}
	if _, denied := allowToolHook(true, nil).Denied(); denied {
		t.Fatal("an escalating hook denies the call")
	}
	if !allowToolHook(true, nil).RequiresApproval() {
		t.Fatal("an escalating hook does not require approval")
	}
}
