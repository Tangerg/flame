package coordination

import (
	"strings"
	"testing"
)

func TestPromptAdmissionHonorsCapturedWorkflowBudget(t *testing.T) {
	valid := Prompt{Operation: OperationExtract, SystemPrompt: "system", UserPrompt: "user", MaxInputBytes: MaximumModelInputBytes, MaxOutputTokens: MaximumModelOutputTokens}
	for _, operation := range []string{OperationExtract, OperationCurate} {
		prompt := valid
		prompt.Operation = operation
		if err := prompt.Validate(); err != nil {
			t.Fatalf("valid workflow operation %q: %v", operation, err)
		}
	}
	for name, change := range map[string]func(*Prompt){
		"foreign operation":            func(prompt *Prompt) { prompt.Operation = "runtime.execute" },
		"embedding through completion": func(prompt *Prompt) { prompt.Operation = "memory.embed" },
		"input grant":                  func(prompt *Prompt) { prompt.MaxInputBytes++ },
		"output grant":                 func(prompt *Prompt) { prompt.MaxOutputTokens++ },
		"empty input grant":            func(prompt *Prompt) { prompt.MaxInputBytes = 0 },
		"empty output grant": func(prompt *Prompt) {
			prompt.MaxOutputTokens = 0
		},
		"combined prompt size": func(prompt *Prompt) {
			prompt.MaxInputBytes = len(prompt.SystemPrompt)
			prompt.UserPrompt = strings.Repeat("界", 3)
		},
	} {
		t.Run(name, func(t *testing.T) {
			prompt := valid
			change(&prompt)
			if err := prompt.Validate(); err == nil {
				t.Fatal("prompt escaped its captured workflow budget")
			}
		})
	}
}
