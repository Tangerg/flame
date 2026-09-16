package agentexec

import (
	"strings"
	"testing"

	corechat "github.com/Tangerg/scope/core/chat"
)

func TestModelContextCompactionRequiresExactModelSelection(t *testing.T) {
	_, err := NewTransientModelContextCompaction(ModelContextCompactionInput{
		SessionID: "session_1",
		Candidate: []corechat.Message{corechat.NewUserMessage(corechat.NewTextPart("compact this"))},
	})
	if err == nil || !strings.Contains(err.Error(), "model selection is required") {
		t.Fatalf("NewTransientModelContextCompaction without model selection error = %v", err)
	}
}
