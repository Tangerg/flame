package promptsource

import (
	"context"
	"errors"
	"fmt"
	"io"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/cancelread"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

func readAuthoredPromptFile(ctx context.Context, path string) ([]byte, error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	file, opened, err := fileinput.Open(path, workspaceapp.MaxAuthoredPromptDocumentBytes)
	if err != nil {
		switch {
		case errors.Is(err, fileinput.ErrNotRegular), errors.Is(err, fileinput.ErrChanged):
			return nil, fmt.Errorf("%w: %q: %w", workspaceapp.ErrInvalidPromptSource, path, err)
		case errors.Is(err, fileinput.ErrTooLarge):
			return nil, fmt.Errorf("%w: %q: %w", workspaceapp.ErrPromptSourceTooLarge, path, err)
		}
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	document, readErr := io.ReadAll(io.LimitReader(
		cancelread.Reader(ctx, file),
		workspaceapp.MaxAuthoredPromptDocumentBytes+1,
	))
	verifyErr := fileinput.VerifyPathVersion(file, opened, path)
	if errors.Is(verifyErr, fileinput.ErrChanged) {
		verifyErr = fmt.Errorf("%w: %w", workspaceapp.ErrInvalidPromptSource, verifyErr)
	}
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("read %q: %w", path, errors.Join(readErr, verifyErr, closeErr))
	}
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	if err := workspaceapp.ValidateAuthoredPromptDocument(document); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if verifyErr != nil {
		return nil, fmt.Errorf("read %q: %w", path, verifyErr)
	}
	return document, nil
}
