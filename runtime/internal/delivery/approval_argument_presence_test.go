package delivery

import (
	"encoding/json/v2"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestApprovalArgumentOverrideSurvivesBindingEncoding(t *testing.T) {
	for _, test := range []struct {
		arguments map[string]any
		want      string
	}{
		{nil, ""},
		{map[string]any{}, "{}"},
		{map[string]any{"target": "chosen"}, `{"target":"chosen"}`},
	} {
		wire := protocol.InterruptResponseValue{
			Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove, EditedArgs: test.arguments,
		}
		encoded, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		var remote protocol.InterruptResponseValue
		if err := json.Unmarshal(encoded, &remote); err != nil {
			t.Fatal(err)
		}
		for _, input := range []protocol.InterruptResponseValue{wire, remote} {
			approval, err := decodeApprovalResponse(input)
			if err != nil {
				t.Fatal(err)
			}
			if !approval.Approved || approval.Arguments != test.want {
				t.Fatalf("approved arguments = %q, want %q", approval.Arguments, test.want)
			}
		}
	}
}
