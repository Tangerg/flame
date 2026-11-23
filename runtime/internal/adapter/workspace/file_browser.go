package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"

	"github.com/Tangerg/scope/tools/textread"
)

type FileBrowser struct{}

func (FileBrowser) List(ctx context.Context, root string, options workspaceapp.FileListOptions) ([]workspaceapp.FileEntry, error) {
	scope, err := resolveFileSelection(root, options.Path)
	if err != nil {
		return nil, err
	}
	if !scope.directory {
		return nil, fmt.Errorf("%w: %q is not a directory", workspaceapp.ErrInvalidFileListPath, options.Path)
	}
	return scope.list(ctx, options)
}

func openFailure(path string, err error) error {
	switch {
	case errors.Is(err, filesystem.ErrNotRegular):
		return fmt.Errorf("%w: %s: %w", workspaceapp.ErrUnsupportedFile, path, err)
	case errors.Is(err, filesystem.ErrFileTooLarge):
		return fmt.Errorf("%w: %s: %w", workspaceapp.ErrFileReadTooLarge, path, err)
	default:
		return err
	}
}

func scanFailure(path string, err error) error {
	switch {
	case errors.Is(err, textread.ErrInputTooLarge):
		return fmt.Errorf("%w: %s grew while reading: %w", workspaceapp.ErrFileReadTooLarge, path, err)
	case errors.Is(err, textread.ErrLineTooLarge):
		return fmt.Errorf("%w: %s line %d exceeds the %d MiB limit: %w",
			workspaceapp.ErrFileReadTooLarge, path, textread.LineNumber(err), workspaceapp.MaxFileReadLineBytes/filesystem.MiB, err)
	case errors.Is(err, textread.ErrInvalidText):
		return fmt.Errorf("%w: %s: %w", workspaceapp.ErrUnsupportedFile, path, err)
	default:
		return fmt.Errorf("workspace: scan %s: %w", path, err)
	}
}

func (FileBrowser) Read(ctx context.Context, root string, input workspaceapp.FileReadPlan) (_ workspaceapp.FileReadResult, err error) {
	if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
		return workspaceapp.FileReadResult{}, cause
	}
	path := input.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: resolve read root: %w", err)
	}
	rootHandle, err := os.OpenRoot(canonicalRoot)
	if err != nil {
		return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: open read root: %w", err)
	}
	defer func() {
		err = errors.Join(err, rootHandle.Close())
	}()
	relative, err := rootRelativeFilePath(root, canonicalRoot, path)
	if err != nil {
		return workspaceapp.FileReadResult{}, err
	}
	file, opened, err := filesystem.OpenFileAt(
		rootHandle,
		filepath.FromSlash(relative),
		workspaceapp.MaxFileReadSourceBytes,
	)
	if err != nil {
		return workspaceapp.FileReadResult{}, openFailure(input.Path, err)
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()

	start, lines := 0, 0
	if input.StartLine > 0 {
		start = input.StartLine - 1
		if input.EndLine >= input.StartLine {
			lines = input.EndLine - input.StartLine + 1
		}
	}
	source := &fileScanInput{reader: file}
	result, err := scanFileWindow(ctx, source, input, start, lines)
	err = errors.Join(err, source.readErr, errors.Join(ctx.Err(), context.Cause(ctx)))
	if err != nil {
		return workspaceapp.FileReadResult{}, scanFailure(input.Path, err)
	}
	if err := filesystem.VerifyAtVersion(file, opened, rootHandle, filepath.FromSlash(relative)); err != nil {
		return workspaceapp.FileReadResult{}, fmt.Errorf("workspace: verify %s after reading: %w", input.Path, err)
	}
	if input.StartLine > result.TotalLines {
		return workspaceapp.FileReadResult{}, workspaceapp.ErrInvalidFileRange
	}
	return workspaceapp.FileReadResult{
		Content: result.Content, TotalLines: result.TotalLines, StartLine: result.Offset,
		EndLine: result.Offset + result.Lines, Truncated: result.Truncated(),
	}, nil
}

func scanFileWindow(ctx context.Context, source io.Reader, input workspaceapp.FileReadPlan, start, lines int) (textread.Result, error) {
	if !input.FromEnd {
		return textread.Scan(ctx, source, textread.Options{
			InputBytes: workspaceapp.MaxFileReadSourceBytes, LineBytes: workspaceapp.MaxFileReadLineBytes,
			OutputBytes: input.MaxBytes, Offset: start, MaxLines: lines,
		})
	}

	var content bytes.Buffer
	result := textread.Result{Offset: start}
	err := textread.VisitLines(ctx, source, textread.Limits{
		InputBytes: workspaceapp.MaxFileReadSourceBytes, LineBytes: workspaceapp.MaxFileReadLineBytes,
	}, func(number int, line []byte) error {
		result.TotalLines = number
		if number <= start || (lines > 0 && number > start+lines) {
			return nil
		}
		if len(line) > input.MaxBytes {
			content.Reset()
			result.Offset, result.Lines = number-1, 0
			return nil
		}
		for result.Lines > 0 && content.Len()+1+len(line) > input.MaxBytes {
			if result.Lines == 1 {
				content.Reset()
			} else {
				content.Next(bytes.IndexByte(content.Bytes(), '\n') + 1)
			}
			result.Lines--
			result.Offset++
		}
		if result.Lines == 0 {
			result.Offset = number - 1
		} else {
			content.WriteByte('\n')
		}
		content.Write(line)
		result.Lines++
		return nil
	})
	if err != nil {
		return textread.Result{}, err
	}
	result.Content = content.String()
	return result, nil
}
