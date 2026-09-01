package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestSessionJSONPreservesReasoningSelection(t *testing.T) {
	t.Parallel()
	session := agent.Session{
		ID: "ses_1", Status: protocol.SessionStatusIdle,
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh",
		Workspace: workspace.Workspace{Path: "/workspace", ProjectRoot: "/workspace", Availability: workspace.Available},
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

	run := agent.Run{
		ID: "run_1", SessionID: "session_1", Status: protocol.RunStatusRunning, ActiveSegmentID: "segment_1",
		Provider: "openai", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh",
		ContextTokens: 32_768,
		Lineage:       agent.RootRunLineage(),
		Limits:        agent.UnlimitedRunLimits(),
		ProtocolProfile: &protocol.RunProtocolProfile{
			RequiredFeatures: []protocol.RunProtocolFeature{protocol.RunProtocolFeatureSubagents},
			InterruptTypes:   []protocol.InterruptType{protocol.InterruptApproval, protocol.InterruptQuestion},
		},
	}
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
