package toolset

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/flame/runtime/internal/infra/process/procgroup"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/fs"
)

const (
	maxAutoFormatFileBytes       int64 = 8 << 20
	maxAutoFormatDiagnosticBytes       = 64 << 10
	autoFormatProcessWaitDelay         = time.Second
)

var errAutoFormatFileTooLarge = fmt.Errorf(
	"auto-format: file exceeds the %d MiB limit", maxAutoFormatFileBytes>>20)

func withAutoFormat(inner toolcontract.Tool, root *filesystemRoot, executor *fs.LocalExecutor) toolcontract.Tool {
	return decorateCall(inner, func(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
		paths, err := mutationPaths(inner, invocation)
		if err != nil {
			return toolcontract.Result{}, fmt.Errorf("inspect mutation paths before formatting: %w", err)
		}
		out, err := inner.Call(ctx, invocation)
		if err != nil || len(paths) == 0 {
			return out, err
		}
		var failed []string
		for _, path := range paths {
			if formatErr := formatPath(ctx, root, executor, path); formatErr != nil {
				failed = append(failed, formatErr.Error())
			}
		}
		if len(failed) == 0 {
			return out, nil
		}
		return toolcontract.Result{Output: appendToolOutputText(out.Output, "\n\nAuto-format skipped or failed:\n- "+strings.Join(failed, "\n- "))}, nil
	})
}

func formatPath(ctx context.Context, root *filesystemRoot, executor *fs.LocalExecutor, path string) error {
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	path, err := rootRelative(root.Name(), path)
	if err != nil {
		return err
	}
	info, err := root.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: inspect before formatting: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	extension := strings.ToLower(filepath.Ext(path))
	prettier := ""
	switch extension {
	case ".go", ".json":
	case ".js", ".jsx", ".ts", ".tsx", ".css", ".scss", ".html", ".md", ".yaml", ".yml":
		prettier, err = exec.LookPath("prettier")
		if errors.Is(err, exec.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: locate prettier: %w", path, err)
		}
	default:
		return nil
	}

	source, err := readAutoFormatFile(ctx, root, executor, path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	input := []byte(source.content)
	var formatted []byte
	switch extension {
	case ".go":
		formatted, err = format.Source(input)
		if err != nil {
			return fmt.Errorf("%s: gofmt: %w", path, err)
		}
	case ".json":
		indented := jsontext.Value(bytes.TrimSpace(input))
		if indentErr := indented.Indent(jsontext.WithIndent("  ")); indentErr != nil {
			return nil
		}
		formatted = append(indented, '\n')
	default:
		if err := verifyWorkspacePath(root); err != nil {
			return err
		}
		formatted, err = runFormatter(ctx, input, prettier, "--stdin-filepath", filepath.Join(root.Name(), path))
		if err != nil {
			return err
		}
	}
	if len(formatted) > int(maxAutoFormatFileBytes) {
		return fmt.Errorf("%s: %w: formatted output uses %d bytes", path, errAutoFormatFileTooLarge, len(formatted))
	}
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	if bytes.Equal(input, formatted) {
		return nil
	}
	return applyFormattedFile(ctx, root, executor, path, formatted, source)
}

func runFormatter(ctx context.Context, input []byte, name string, args ...string) ([]byte, error) {
	if err := cancellation.Err(ctx); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	stdout := capture.NewWriter(int(maxAutoFormatFileBytes))
	stderr := capture.NewWriter(maxAutoFormatDiagnosticBytes)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = autoFormatProcessWaitDelay
	procgroup.Prepare(cmd)
	cmd.Cancel = func() error { return procgroup.Stop(cmd) }
	runErr := cmd.Run()
	cleanupErr := procgroup.Stop(cmd)
	if errors.Is(cleanupErr, os.ErrProcessDone) {
		cleanupErr = nil
	}
	failure := errors.Join(runErr, cleanupErr, cancellation.Err(ctx))
	target := name
	if len(args) > 0 {
		target = args[len(args)-1]
	}
	if stdout.Truncated() {
		failure = errors.Join(failure, fmt.Errorf("%s output: %w", name, errAutoFormatFileTooLarge))
	}
	if stderr.Truncated() {
		failure = errors.Join(failure, fmt.Errorf("%s diagnostic output exceeded %d bytes", name, maxAutoFormatDiagnosticBytes))
	}
	if failure == nil {
		return bytes.Clone(stdout.Bytes()), nil
	}
	msg := strings.TrimSpace(stderr.Diagnostic())
	if msg != "" && stderr.Truncated() {
		msg += "\n... [formatter diagnostic truncated] ..."
	}
	if msg == "" {
		return nil, fmt.Errorf("%s: run %s: %w", target, name, failure)
	}
	return nil, fmt.Errorf("%s: %s: %w", target, msg, failure)
}

type autoFormatSource struct {
	content string
	info    os.FileInfo
}

func readAutoFormatFile(ctx context.Context, root *filesystemRoot, executor *fs.LocalExecutor, path string) (autoFormatSource, error) {
	if err := cancellation.Err(ctx); err != nil {
		return autoFormatSource{}, err
	}
	before, err := root.Lstat(path)
	if err != nil {
		return autoFormatSource{}, err
	}
	if err := validateAutoFormatSource(before); err != nil {
		return autoFormatSource{}, err
	}
	result, err := executor.Read(ctx, fs.ReadInput{
		Path: path, MaxInputBytes: maxAutoFormatFileBytes,
		MaxLineBytes: int(maxAutoFormatFileBytes), MaxOutputBytes: int(maxAutoFormatFileBytes),
	})
	if err != nil {
		return autoFormatSource{}, errors.Join(err, cancellation.Err(ctx))
	}
	if result.Truncated {
		return autoFormatSource{}, errors.Join(errAutoFormatFileTooLarge, cancellation.Err(ctx))
	}
	after, err := root.Lstat(path)
	if err != nil || !filesystem.SameVersion(before, after) {
		return autoFormatSource{}, errors.Join(errors.New("file changed while reading for formatting"), err, cancellation.Err(ctx))
	}
	if err := cancellation.Err(ctx); err != nil {
		return autoFormatSource{}, err
	}
	return autoFormatSource{content: result.Content, info: after}, nil
}

func validateAutoFormatSource(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported file mode %s", info.Mode().Type())
	}
	if info.Size() > maxAutoFormatFileBytes {
		return fmt.Errorf("%w: file uses %d bytes", errAutoFormatFileTooLarge, info.Size())
	}
	return nil
}

// Scope owns atomic replacement, parent-directory handles, and preservation of
// BOM, line endings, and mode. Formatting supplies only an exact-text edit of
// the complete, bounded source it observed.
func applyFormattedFile(ctx context.Context, root *filesystemRoot, executor *fs.LocalExecutor, path string, data []byte, source autoFormatSource) error {
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	current, err := root.Lstat(path)
	if err != nil || !filesystem.SameVersion(source.info, current) {
		return errors.Join(errors.New("file changed while formatting"), err, cancellation.Err(ctx))
	}
	_, err = executor.Edit(ctx, fs.EditRequest{Path: path, OldString: source.content, NewString: string(data)})
	if err != nil {
		return errors.Join(err, cancellation.Err(ctx))
	}
	return nil
}
