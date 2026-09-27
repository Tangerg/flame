package prompt

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRunOptionsValidateBounds(t *testing.T) {
	temperature, topP, maxTokens := 0.7, 0.9, int64(4096)
	options := RunOptions{
		Provider: "mock", Model: "balanced", ReasoningEffort: "high",
		Generation: protocol.GenerationParams{
			Temperature: &temperature, TopP: &topP, MaxTokens: &maxTokens, Stop: []string{"END"},
		},
	}
	if err := options.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := 3.0
	options.Generation.Temperature = &bad
	if err := options.Validate(); err == nil {
		t.Fatal("invalid temperature was accepted")
	}
	options.Generation = protocol.GenerationParams{Stop: []string{"END", "END"}}
	if err := options.Validate(); err == nil {
		t.Fatal("duplicate stop sequences were accepted")
	}
	options = RunOptions{ReasoningEffort: "high"}
	if err := options.Validate(); err == nil {
		t.Fatal("reasoning effort without a model was accepted")
	}
}

func TestRunOptionsEqualPreservesOptionalGenerationSemantics(t *testing.T) {
	zero := 0.0
	left := RunOptions{
		Provider: "deepseek", Model: "v4",
		Generation: protocol.GenerationParams{Temperature: &zero, Stop: []string{"END"}},
	}
	if !left.Equal(left.Clone()) {
		t.Fatal("cloned options are not equal")
	}
	right := left.Clone()
	right.Generation.Temperature = nil
	if left.Equal(right) {
		t.Fatal("explicit zero temperature equals an omitted temperature")
	}
	right = left.Clone()
	right.Generation.Stop[0] = "STOP"
	if left.Equal(right) {
		t.Fatal("different stop sequences are equal")
	}
	right = left.Clone()
	right.ReasoningEffort = "high"
	if left.Equal(right) {
		t.Fatal("different reasoning effort is equal")
	}
}
