package input

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestRequireRejectsEmptyKeyBeforeParking(t *testing.T) {
	prompt := runs.Interrupt{
		Kind: run.InterruptQuestion,
		Question: &runs.QuestionPrompt{
			ToolName:  "ask_user",
			Arguments: "{}",
			Fields:    []runs.QuestionFieldSpec{{Prompt: "Continue?"}},
		},
	}
	ctx := WithCapabilities(t.Context(), []run.InterruptKind{run.InterruptQuestion})
	for _, key := range []string{"", " \t"} {
		if _, _, err := Require(ctx, toolcontract.Invocation{}, key, prompt); err == nil || !strings.Contains(err.Error(), "request key is required") {
			t.Fatalf("Require(%q) error = %v, want required-key error", key, err)
		}
	}
}
