package agentexec

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	corechat "github.com/Tangerg/scope/core/chat"
)

func TestModelContextCompactionRequiresExactModelSelection(t *testing.T) {
	_, err := NewTransientModelContextCompaction(
		"session_1",
		modelref.Selection{},
		nil,
		[]corechat.Message{corechat.NewUserMessage(corechat.NewTextPart("compact this"))},
		nil,
		corechat.Options{},
		ModelContextTokenCalibration{},
		nil,
		0,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "model selection is required") {
		t.Fatalf("NewTransientModelContextCompaction without model selection error = %v", err)
	}
}
