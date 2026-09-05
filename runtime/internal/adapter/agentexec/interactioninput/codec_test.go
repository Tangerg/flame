package interactioninput

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func TestDecodePromptDiscriminatesAndRejectsGuesses(t *testing.T) {
	question := runs.Interrupt{
		Kind: interrupt.Question,
		Question: &runs.QuestionPrompt{
			CallID:    "tool_question_1",
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
	if !strings.Contains(encoded, `"callId":"tool_question_1"`) ||
		!strings.Contains(encoded, `"fields"`) || !strings.Contains(encoded, `"prompt"`) ||
		strings.Contains(encoded, `"multiSelect"`) {
		t.Fatalf("question checkpoint uses stale vocabulary: %s", encoded)
	}
	got, err := DecodePrompt(raw)
	if err != nil {
		t.Fatalf("DecodePrompt: %v", err)
	}
	if got.Kind != interrupt.Question || got.Question == nil || got.Question.CallID != "tool_question_1" ||
		got.Question.ToolName != "ask_user" ||
		!got.Question.Fields[0].AllowCustom {
		t.Fatalf("decoded = %#v", got)
	}

	approval := runs.Interrupt{
		Kind: interrupt.Approval,
		Approval: &runs.ApprovalPrompt{
			CallID: "tool_approval_1", ToolName: "web_fetch", Arguments: `{"url":"https://example.com"}`,
			SafetyClass: tool.SafetyClassNetwork, Risk: tool.RiskHigh,
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

func TestResolutionCodecUsesAgentWireVocabulary(t *testing.T) {
	raw, err := EncodeResolution(interrupt.Resolution{
		Approved: true, Arguments: `{"command":"go test","description":"Run tests"}`, Answers: [][]string{{"yes"}},
		RememberScope: approval.ScopeSession,
	})
	if err != nil {
		t.Fatalf("EncodeResolution: %v", err)
	}
	var wire map[string]any
	if unmarshalErr := json.Unmarshal(raw, &wire); unmarshalErr != nil {
		t.Fatalf("decode encoded response: %v", unmarshalErr)
	}
	if wire["approved"] != true || wire["remember_scope"] != "session" || wire["answers"] == nil {
		t.Fatalf("response wire = %#v", wire)
	}
	if _, found := wire["Approved"]; found {
		t.Fatalf("response leaked Go field name: %#v", wire)
	}
	if _, decodeResolutionErr := DecodeResolution([]byte(`{"Answers":[]}`)); decodeResolutionErr == nil {
		t.Fatal("DecodeResolution accepted a case-insensitive Go field name")
	}
	if _, decodeResolutionErr := DecodeResolution([]byte(`{"approved":true,"approved":false}`)); decodeResolutionErr == nil {
		t.Fatal("DecodeResolution accepted a duplicate field")
	}
	for _, missing := range [][]byte{
		[]byte(`{}`),
		[]byte(`{"approved":null}`),
	} {
		if _, decodeResolutionErr := DecodeResolution(missing); decodeResolutionErr == nil {
			t.Fatalf("DecodeResolution(%s) accepted a missing approval decision", missing)
		}
	}
	denied, err := DecodeResolution([]byte(`{"approved":false,"reason":"not now"}`))
	if err != nil || denied.Approved || denied.Reason != "not now" {
		t.Fatalf("DecodeResolution explicit denial = %#v, %v", denied, err)
	}
	decoded, err := DecodeResolution(raw)
	if err != nil || decoded.RememberScope != approval.ScopeSession ||
		len(decoded.Answers) != 1 || len(decoded.Answers[0]) != 1 || decoded.Answers[0][0] != "yes" {
		t.Fatalf("DecodeResolution = %#v, %v", decoded, err)
	}
}

func TestContinuationCodecRejectsAliasesAndDuplicates(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"Key":"approval.shell"}`),
		[]byte(`{"key":"first","key":"second"}`),
	} {
		var continuation continuationWire
		if err := decode(raw, &continuation); err == nil {
			t.Errorf("decode(%s) succeeded, want error", raw)
		}
	}
}
