package coordination

import (
	"strings"
	"testing"
)

func TestPromptAdmissionHonorsWorkflowBudget(t *testing.T) {
	valid := Prompt{SystemPrompt: "system", UserPrompt: "user", MaxOutputTokens: MaximumModelOutputTokens}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid completion prompt: %v", err)
	}
	boundary := valid
	boundary.UserPrompt = strings.Repeat("x", MaximumModelInputBytes-len(boundary.SystemPrompt))
	if err := boundary.Validate(); err != nil {
		t.Fatalf("prompt at the input limit: %v", err)
	}
	for name, change := range map[string]func(*Prompt){
		"output allocation above the workflow limit": func(prompt *Prompt) { prompt.MaxOutputTokens++ },
		"empty output allocation": func(prompt *Prompt) {
			prompt.MaxOutputTokens = 0
		},
		"combined prompt size": func(prompt *Prompt) {
			prompt.UserPrompt = strings.Repeat("x", MaximumModelInputBytes)
		},
	} {
		t.Run(name, func(t *testing.T) {
			prompt := valid
			change(&prompt)
			if err := prompt.Validate(); err == nil {
				t.Fatal("prompt escaped its workflow budget")
			}
		})
	}
}
