package promptsource

import (
	"context"
	"errors"
	"testing"
)

func TestReadAuthoredPromptFilePreservesCancellationCause(t *testing.T) {
	cause := errors.New("authored prompt canceled")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)

	if _, err := readAuthoredPromptFile(ctx, "unused"); !errors.Is(err, cause) {
		t.Fatalf("readAuthoredPromptFile error = %v, want cancellation cause", err)
	}
}
