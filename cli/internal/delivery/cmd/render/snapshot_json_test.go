package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSessionJSONPreservesReasoningSelection(t *testing.T) {
	t.Parallel()
	session := protocol.Session{
		ID: "ses_1", Status: protocol.SessionStatusIdle,
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh",
		Workspace: protocol.WorkspaceInfo{Ref: protocol.WorkspaceRef{Path: "/workspace"}, ProjectRoot: "/workspace", Availability: protocol.WorkspaceAvailable},
		Revision:  1,
	}
	var output bytes.Buffer
	if err := WriteSessionJSON(&output, session); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"reasoningEffort":"xhigh"`) {
		t.Fatalf("session JSON omitted reasoning effort: %s", output.String())
	}
}

func TestRunJSONPreservesNegotiatedProtocolProfile(t *testing.T) {
	t.Parallel()

	run := protocol.RunRef{RunSummary: protocol.RunSummary{ID: "run_1", SessionID: "session_1", Status: protocol.RunStatusRunning, Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh"}, ActiveSegmentID: "segment_1", ContextTokens: 32_768, ProtocolProfile: protocol.RunProtocolProfile{
		RequiredFeatures: []protocol.RunProtocolFeature{protocol.RunProtocolFeatureSubagents},
		InterruptTypes:   []protocol.InterruptType{protocol.InterruptApproval, protocol.InterruptQuestion},
	}}
	var output bytes.Buffer
	if err := WriteRunJSON(&output, run); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"reasoningEffort":"xhigh"`, `"contextTokens":32768`, `"protocolProfile"`,
		`"requiredFeatures":["subagents"]`, `"interruptTypes":["approval","question"]`,
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("run JSON omitted %s: %s", want, output.String())
		}
	}
}
