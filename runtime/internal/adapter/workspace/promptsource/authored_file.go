package promptsource

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Tangerg/flame/packages/go-sdk/cancellation"
	"github.com/Tangerg/flame/packages/go-sdk/fileinput"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

func readAuthoredPromptFile(ctx context.Context, path string) ([]byte, error) {
	if cause := cancellation.Err(ctx); cause != nil {
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
		cancellation.Reader(ctx, file),
		workspaceapp.MaxAuthoredPromptDocumentBytes+1,
	))
	verifyErr := fileinput.VerifyPathVersion(file, opened, path)
	if errors.Is(verifyErr, fileinput.ErrChanged) {
		verifyErr = fmt.Errorf("%w: %w", workspaceapp.ErrInvalidPromptSource, verifyErr)
	}
	closeErr := file.Close()
	var admissionErr error
	if readErr == nil {
		admissionErr = workspaceapp.ValidateAuthoredPromptDocument(document)
	} else {
		// A failed read establishes size, but cannot establish text validity.
		admissionErr = workspaceapp.ValidateAuthoredPromptDocumentSize(int64(len(document)))
	}
	if err := errors.Join(readErr, admissionErr, verifyErr, closeErr, cancellation.Err(ctx)); err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}
	return document, nil
}
