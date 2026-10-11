package promptsource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

func TestReadAuthoredPromptFilePreservesCancellationCause(t *testing.T) {
	cause := errors.New("authored prompt canceled")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)

	if _, err := readAuthoredPromptFile(ctx, "unused"); !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("readAuthoredPromptFile error = %v, want cancellation category and owner cause", err)
	}
}

func TestAuthoredPromptAdmissionPreservesInputFailure(t *testing.T) {
	for _, test := range []struct {
		name     string
		category error
		cause    error
	}{
		{"directory", workspaceapp.ErrInvalidPromptSource, fileinput.ErrNotRegular},
		{"oversized", workspaceapp.ErrPromptSourceTooLarge, fileinput.ErrTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "AGENTS.md")
			var err error
			if test.name == "directory" {
				err = os.Mkdir(path, 0700)
			} else {
				err = os.WriteFile(path, make([]byte, workspaceapp.MaxAuthoredPromptDocumentBytes+1), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			content, err := readAuthoredPromptFile(t.Context(), path)
			if len(content) != 0 || !errors.Is(err, test.category) || !errors.Is(err, test.cause) {
				t.Fatalf("prompt admission lost classification or input cause: %v", err)
			}
		})
	}
}
