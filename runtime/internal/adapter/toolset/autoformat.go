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

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
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
	return decorateCall(inner, func(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
		paths, err := mutationPaths(inner, invocation)
		if err != nil {
			return chat.ToolOutput{}, fmt.Errorf("inspect mutation paths before formatting: %w", err)
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
		return appendToolOutputText(out, "\n\nAuto-format skipped or failed:\n- "+strings.Join(failed, "\n- ")), nil
	})
}

func formatPath(ctx context.Context, root *filesystemRoot, executor *fs.LocalExecutor, path string) error {
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
		if err != nil {
			return nil
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
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if bytes.Equal(input, formatted) {
		return nil
	}
	return applyFormattedFile(ctx, root, executor, path, formatted, source)
}

func runFormatter(ctx context.Context, input []byte, name string, args ...string) ([]byte, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	stdout := &formatOutputBuffer{limit: int(maxAutoFormatFileBytes)}
	stderr := &formatOutputBuffer{limit: maxAutoFormatDiagnosticBytes}
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = autoFormatProcessWaitDelay
	runErr := cmd.Run()
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	target := name
	if len(args) > 0 {
		target = args[len(args)-1]
	}
	if stdout.overflow {
		return nil, fmt.Errorf("%s: %s output exceeds %d MiB", target, name, maxAutoFormatFileBytes>>20)
	}
	if runErr == nil && !stderr.overflow {
		return bytes.Clone(stdout.Bytes()), nil
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		if runErr == nil {
			return nil, fmt.Errorf("%s: %s diagnostic output was truncated", target, name)
		}
		return nil, fmt.Errorf("%s: run %s: %w", target, name, runErr)
	}
	if runErr == nil {
		return nil, fmt.Errorf("%s: %s", target, msg)
	}
	return nil, fmt.Errorf("%s: %s: %w", target, msg, runErr)
}

type autoFormatSource struct {
	content string
	info    os.FileInfo
}

func readAutoFormatFile(ctx context.Context, root *filesystemRoot, executor *fs.LocalExecutor, path string) (autoFormatSource, error) {
	if err := context.Cause(ctx); err != nil {
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
		return autoFormatSource{}, err
	}
	if result.Truncated {
		return autoFormatSource{}, errAutoFormatFileTooLarge
	}
	after, err := root.Lstat(path)
	if err != nil || !fileinput.SameVersion(before, after) {
		return autoFormatSource{}, errors.New("file changed while reading for formatting")
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

type formatOutputBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (f *formatOutputBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := f.limit - f.buffer.Len()
	if remaining > 0 {
		_, _ = f.buffer.Write(value[:min(len(value), remaining)])
	}
	if len(value) > remaining {
		f.overflow = true
	}
	return written, nil
}

func (f *formatOutputBuffer) String() string {
	if f.overflow {
		return f.buffer.String() + "\n... [formatter diagnostic truncated] ..."
	}
	return f.buffer.String()
}

func (f *formatOutputBuffer) Bytes() []byte { return f.buffer.Bytes() }

// Scope owns atomic replacement, parent-directory handles, and preservation of
// BOM, line endings, and mode. Formatting supplies only an exact-text edit of
// the complete, bounded source it observed.
func applyFormattedFile(ctx context.Context, root *filesystemRoot, executor *fs.LocalExecutor, path string, data []byte, source autoFormatSource) error {
	current, err := root.Lstat(path)
	if err != nil || !fileinput.SameVersion(source.info, current) {
		return errors.New("file changed while formatting")
	}
	_, err = executor.Edit(ctx, fs.EditRequest{Path: path, OldString: source.content, NewString: string(data)})
	return err
}
