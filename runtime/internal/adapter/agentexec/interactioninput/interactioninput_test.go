package interactioninput

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
)

func TestRequireRejectsEmptyKeyBeforeParking(t *testing.T) {
	prompt := runs.Interrupt{
		Kind: interrupt.Question,
		Question: &runs.QuestionPrompt{
			ToolName:  "ask_user",
			Arguments: "{}",
			Fields:    []runs.QuestionFieldSpec{{Prompt: "Continue?"}},
		},
	}
	ctx := WithCapabilities(t.Context(), []interrupt.Kind{interrupt.Question})
	for _, key := range []string{"", " \t"} {
		if _, err := Require(ctx, key, prompt); err == nil || !strings.Contains(err.Error(), "request key is required") {
			t.Fatalf("Require(%q) error = %v, want required-key error", key, err)
		}
	}
}
