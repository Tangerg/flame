package main

import (
	"strings"
	"testing"

	app "github.com/Tangerg/flame/plugins/memory/internal/application"
)

func TestRemoteCompletionRefusesOversizedPromptsBeforeCallback(t *testing.T) {
	_, err := (remoteModels{}).Complete(t.Context(), app.Prompt{
		SystemPrompt: "system", UserPrompt: strings.Repeat("x", app.MaximumModelInputBytes),
		MaxOutputTokens: app.MaximumModelOutputTokens,
	})
	if err == nil || !strings.Contains(err.Error(), "completion prompt refused") {
		t.Fatalf("oversized prompt reached callback setup: %v", err)
	}
}
