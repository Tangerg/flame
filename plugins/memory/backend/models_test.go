package main

import (
	"strings"
	"testing"

	app "github.com/Tangerg/flame/plugins/memory/internal/application"
)

func TestRemoteCompletionRefusesForeignOperationsBeforeCallback(t *testing.T) {
	for _, operation := range []string{"runtime.execute", "memory.embed"} {
		t.Run(operation, func(t *testing.T) {
			// An empty grant makes accidental callback use observable as a different
			// error. Operation admission belongs to the workflow before any I/O.
			_, err := (remoteModels{}).Complete(t.Context(), app.Prompt{
				Operation: operation, UserPrompt: "captured input",
				MaxInputBytes: app.MaximumModelInputBytes, MaxOutputTokens: app.MaximumModelOutputTokens,
			})
			if err == nil || !strings.Contains(err.Error(), "completion prompt refused") {
				t.Fatalf("foreign operation reached callback setup: %v", err)
			}
		})
	}
}
