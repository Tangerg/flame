package input

import (
	json "encoding/json/v2"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/agent"
)

func TestDecodePromptDiscriminatesAndRejectsGuesses(t *testing.T) {
	question := runs.Interrupt{
		Kind: interrupt.Question,
		Question: &runs.QuestionPrompt{
			ToolName:  "ask_user",
			Arguments: `{"questions":[{"question":"Continue?"}]}`,
			Fields: []runs.QuestionFieldSpec{{
				Prompt: "Continue?", AllowCustom: true,
				Options: []runs.QuestionOptionSpec{{Label: "Yes"}, {Label: "No"}},
			}},
		},
	}
	raw, err := json.Marshal(promptWireFrom(question))
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	if !strings.Contains(encoded, `"fields"`) || !strings.Contains(encoded, `"prompt"`) ||
		strings.Contains(encoded, `"multiSelect"`) {
		t.Fatalf("question checkpoint uses stale vocabulary: %s", encoded)
	}
	got, err := DecodePrompt(raw)
	if err != nil {
		t.Fatalf("DecodePrompt: %v", err)
	}
	if got.Kind != interrupt.Question || got.Question == nil ||
		got.Question.ToolName != "ask_user" ||
		!got.Question.Fields[0].AllowCustom {
		t.Fatalf("decoded = %#v", got)
	}

	approval := runs.Interrupt{
		Kind: interrupt.Approval,
		Approval: &runs.ApprovalPrompt{
			CallID: "tool_approval_1", Arguments: `{"url":"https://example.com"}`,
			SafetyClass: tool.SafetyClassNetwork, Risk: tool.RiskHigh, Tool: testsupport.BuiltInTool(t, "web_fetch"), SourceFingerprint: testsupport.ToolFingerprint(testsupport.BuiltInTool(t, "web_fetch")),
		},
	}
	raw, err = json.Marshal(promptWireFrom(approval))
	if err != nil {
		t.Fatal(err)
	}
	got, err = DecodePrompt(raw)
	if err != nil {
		t.Fatalf("DecodePrompt approval: %v", err)
	}
	if got.Approval == nil || got.Approval.SafetyClass != tool.SafetyClassNetwork || got.Approval.Risk != tool.RiskHigh {
		t.Fatalf("decoded approval = %#v", got.Approval)
	}

	for _, raw := range [][]byte{
		[]byte(`{"toolName":"shell","arguments":"{}"}`),
		[]byte(`{"kind":"future","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec"}}`),
		[]byte(`{"kind":"approval","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec"},"question":{"toolName":"ask_user","arguments":"{}","fields":[{"prompt":"x"}]}}`),
		[]byte(`{"kind":"question","question":{"toolName":"ask_user","arguments":"{}","fields":[]}}`),
		[]byte(`{"kind":"question","question":{"toolName":"ask_user","arguments":"{}","questions":[{"question":"x"}]}}`),
		[]byte(`{"kind":"approval","approval":{"toolName":"shell","arguments":"not-json","safetyClass":"exec"}}`),
		[]byte(`{"kind":"approval","approval":{"toolName":"shell","arguments":"{}","safetyClass":"future"}}`),
		[]byte(`{"kind":"approval","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec","risk":"critical"}}`),
		[]byte(`{"Kind":"approval","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec"}}`),
		[]byte(`{"kind":"approval","approval":{"ToolName":"shell","arguments":"{}","safetyClass":"exec"}}`),
		[]byte(`{"kind":"approval","kind":"question","approval":{"toolName":"shell","arguments":"{}","safetyClass":"exec"}}`),
	} {
		if _, err := DecodePrompt(raw); err == nil {
			t.Errorf("DecodePrompt(%s) succeeded, want error", raw)
		}
	}
}

func TestApprovalPromptRequiresIdentityRegardlessOfRememberability(t *testing.T) {
	for _, rememberable := range []bool{false, true} {
		for _, fingerprint := range []fingerprint.Digest{{}, testsupport.Digest("source")} {
			prompt := runs.Interrupt{
				Kind: interrupt.Approval,
				Approval: &runs.ApprovalPrompt{
					CallID: "call_approval", Arguments: `{}`,
					SafetyClass: tool.SafetyClassExec, Risk: tool.RiskHigh,
					Rememberable: rememberable, SourceFingerprint: fingerprint,
				},
			}
			if _, err := EncodePrompt(prompt); err == nil {
				t.Errorf("encoded an approval without a tool reference: rememberable=%v fingerprint=%q", rememberable, fingerprint)
			}
			raw, err := json.Marshal(map[string]any{
				"kind": "approval",
				"approval": map[string]any{
					"callId": "call_approval", "toolName": "shell", "arguments": "{}",
					"safetyClass": "exec", "risk": "high", "rememberable": rememberable,
					"sourceFingerprint": fingerprint,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodePrompt(raw); err == nil {
				t.Errorf("restored an approval without a tool reference: %s", raw)
			}
		}
	}
}

func TestResolutionCodecUsesClosedAnswerVocabulary(t *testing.T) {
	for _, answer := range []interrupt.Resolution{
		testsupport.QuestionAnswer(t, [][]string{{"yes"}, {}}),
		testsupport.ApprovalAnswer(t, approval.Resolution{Decision: approval.Allow, EditedArguments: `{"command":"go test","description":"Run tests"}`, RememberScope: approval.ScopeSession}),
		testsupport.ApprovalAnswer(t, approval.Resolution{Decision: approval.Deny, Reason: "not now"}),
	} {
		raw, err := EncodeResolution(answer)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeResolution(raw)
		if err != nil {
			t.Fatal(err)
		}
		again, err := EncodeResolution(decoded)
		if err != nil || string(raw) != string(again) {
			t.Fatalf("answer bytes changed: %s -> %s: %v", raw, again, err)
		}
	}
	for _, raw := range []string{
		`{}`, `{"approved":true}`, `{"question":null}`, `{"Question":{"answers":[["yes"]]}}`,
		`{"question":{"answers":[["yes"]]},"approval":{"decision":"allow"}}`,
		`{"approval":{"decision":"allow","decision":"deny"}}`,
		`{"approval":{"decision":"deny","arguments":"{}"}}`,
		`{"approval":{"decision":"allow","reason":"not now"}}`,
		`{"question":{"answers":[]}}`,
	} {
		if _, err := DecodeResolution([]byte(raw)); err == nil {
			t.Fatalf("invalid answer accepted: %s", raw)
		}
	}
}

func TestContinuationCodecRejectsAliasesAndDuplicates(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"Key":"approval.shell"}`),
		[]byte(`{"key":"first","key":"second"}`),
	} {
		if _, err := decode[continuationWire](raw); err == nil {
			t.Errorf("decode(%s) succeeded, want error", raw)
		}
	}
}

// TestResolutionSchemaGatesWhatTheCodecReads holds the wait boundary and the
// codec to one contract: the Agent Framework validates a host's response with
// this schema before the codec ever sees it, so a shape the codec requires and
// a shape the schema admits cannot be two different answers.
func TestResolutionSchemaGatesWhatTheCodecReads(t *testing.T) {
	raw, err := resolutionSchema()
	if err != nil {
		t.Fatal(err)
	}
	schema, err := agent.ParseSchema(raw)
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := EncodeResolution(testsupport.QuestionAnswer(t, [][]string{{"yes"}}))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(accepted); err != nil {
		t.Fatalf("schema rejected an encoded resolution: %v", err)
	}
	for name, candidate := range map[string][]byte{
		"missing answer": []byte(`{}`),
		"mixed answer":   []byte(`{"question":{"answers":[["yes"]]},"approval":{"decision":"allow"}}`),
		"unknown member": []byte(`{"question":{"answers":[["yes"]]},"approved":true}`),
		"wrong type":     []byte(`{"question":{"answers":"yes"}}`),
	} {
		if err := schema.Validate(candidate); err == nil {
			t.Errorf("schema accepted %s: %s", name, candidate)
		}
	}
}
