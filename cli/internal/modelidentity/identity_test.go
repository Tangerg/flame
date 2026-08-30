package modelidentity

import (
	"strings"
	"testing"
)

func TestSelectionUsesRuntimeIdentityEnvelope(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider string
		model    string
		effort   string
	}{
		{name: "half pair", provider: "openai"},
		{name: "provider whitespace", provider: "open ai", model: "gpt-5"},
		{name: "model control", provider: "openai", model: "gpt-5\n"},
		{name: "provider too long", provider: strings.Repeat("p", MaximumProviderCharacters+1), model: "gpt-5"},
		{name: "model too long", provider: "openai", model: strings.Repeat("m", MaximumModelCharacters+1)},
		{name: "effort too long", provider: "openai", model: "gpt-5", effort: strings.Repeat("e", MaximumReasoningEffortCharacters+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := Selection(test.provider, test.model, test.effort); err == nil {
				t.Fatal("Selection accepted invalid identity")
			}
		})
	}
	if err := Selection(
		strings.Repeat("提", MaximumProviderCharacters),
		strings.Repeat("模", MaximumModelCharacters),
		strings.Repeat("强", MaximumReasoningEffortCharacters),
	); err != nil {
		t.Fatalf("Selection rejected Unicode boundary: %v", err)
	}
}
