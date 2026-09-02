package promptsource

import (
	"context"
	"errors"
	"fmt"
	"io"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
)

func readAuthoredPromptFile(ctx context.Context, path string) ([]byte, error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	file, _, err := fileinput.Open(path, workspaceapp.MaxAuthoredPromptDocumentBytes)
	if err != nil {
		switch {
		case errors.Is(err, fileinput.ErrNotRegular):
			return nil, fmt.Errorf("%w: %q is not a regular file", workspaceapp.ErrInvalidPromptSource, path)
		case errors.Is(err, fileinput.ErrTooLarge):
			return nil, fmt.Errorf(
				"%s: %w",
				path,
				workspaceapp.ValidateAuthoredPromptDocumentSize(workspaceapp.MaxAuthoredPromptDocumentBytes+1),
			)
		case errors.Is(err, fileinput.ErrChanged):
			return nil, fmt.Errorf("%w: %q changed while it was being opened", workspaceapp.ErrInvalidPromptSource, path)
		}
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	document, readErr := io.ReadAll(io.LimitReader(
		promptContextReader{ctx: ctx, reader: file},
		workspaceapp.MaxAuthoredPromptDocumentBytes+1,
	))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("read %q: %w", path, errors.Join(readErr, closeErr))
	}
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	if err := workspaceapp.ValidateAuthoredPromptDocument(document); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return document, nil
}

type promptContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (p promptContextReader) Read(buffer []byte) (int, error) {
	if cause := context.Cause(p.ctx); cause != nil {
		return 0, cause
	}
	read, err := p.reader.Read(buffer)
	if cause := context.Cause(p.ctx); cause != nil {
		return read, cause
	}
	return read, err
}
