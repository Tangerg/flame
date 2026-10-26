package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/tools/textread"
)

func TestFileBrowserReadOwnsSourceLineAndWindowLimits(t *testing.T) {
	t.Run("source", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "oversized.txt")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if truncateErr := file.Truncate(workspaceapp.MaxFileReadSourceBytes + 1); truncateErr != nil {
			t.Fatal(truncateErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
		_, err = (FileBrowser{}).Read(t.Context(), root, workspaceapp.FileReadPlan{Path: "oversized.txt", MaxBytes: workspaceapp.DefaultFileReadBytes})
		if !errors.Is(err, workspaceapp.ErrFileReadTooLarge) || !errors.Is(err, filesystem.ErrFileTooLarge) {
			t.Fatalf("oversized source error = %v, want ErrFileReadTooLarge", err)
		}
	})

	t.Run("line", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(
			filepath.Join(root, "line.txt"), []byte(strings.Repeat("x", workspaceapp.MaxFileReadLineBytes+1)), 0o644,
		); err != nil {
			t.Fatal(err)
		}
		_, err := (FileBrowser{}).Read(t.Context(), root, workspaceapp.FileReadPlan{Path: "line.txt", MaxBytes: workspaceapp.DefaultFileReadBytes})
		if !errors.Is(err, workspaceapp.ErrFileReadTooLarge) || !errors.Is(err, textread.ErrLineTooLarge) {
			t.Fatalf("oversized line error = %v, want ErrFileReadTooLarge", err)
		}
	})

	t.Run("range", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "short.txt"), []byte("first\nsecond"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := (FileBrowser{}).Read(t.Context(), root, workspaceapp.FileReadPlan{
			Path: "short.txt", StartLine: 3, MaxBytes: workspaceapp.DefaultFileReadBytes,
		})
		if !errors.Is(err, workspaceapp.ErrInvalidFileRange) {
			t.Fatalf("outside range error = %v, want ErrInvalidFileRange", err)
		}
	})
}

func TestFileBrowserReadClipsAtUTF8BoundaryAndPreservesTextShape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "utf8.txt"), []byte("\xef\xbb\xbféé\r\nlast\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := (FileBrowser{}).Read(t.Context(), root, workspaceapp.FileReadPlan{Path: "utf8.txt", MaxBytes: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "é" || got.StartLine != 0 || got.EndLine != 1 || got.TotalLines != 3 || !got.Truncated {
		t.Fatalf("Read = %+v, want one valid UTF-8 prefix and honest normalized shape", got)
	}
}

func TestFileBrowserReadConfinesSymlinksToWorkspace(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "inside.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("nested", "inside.txt"), filepath.Join(root, "inside-link")); err != nil {
		t.Fatal(err)
	}
	read, err := (FileBrowser{}).Read(t.Context(), root, workspaceapp.FileReadPlan{Path: "inside-link", MaxBytes: workspaceapp.DefaultFileReadBytes})
	if err != nil {
		t.Fatal(err)
	}
	if read.Content != "inside" {
		t.Fatalf("confined symlink content = %q, want inside", read.Content)
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.txt"), filepath.Join(root, "outside-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := (FileBrowser{}).Read(
		t.Context(), root, workspaceapp.FileReadPlan{Path: "outside-link", MaxBytes: workspaceapp.DefaultFileReadBytes},
	); !errors.Is(err, workspaceapp.ErrPathOutsideRoot) {
		t.Fatalf("escaping symlink error = %v, want ErrPathOutsideRoot", err)
	}
}

func TestFileBrowserGrepReturnsStableRootRelativePaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("first\nneedle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "bravo.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := (FileBrowser{}).Grep(t.Context(), root, workspaceapp.GrepPlan{Pattern: regexp.MustCompile("needle"), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || len(got.Matches) != 2 {
		t.Fatalf("Grep = %+v, want two matches", got)
	}
	if got.Matches[0].Path != "alpha.txt" || got.Matches[0].LineNumber != 2 || got.Matches[1].Path != "nested/bravo.txt" || got.Matches[1].LineNumber != 1 {
		t.Fatalf("matches = %+v, want stable root-relative alpha then nested/bravo", got.Matches)
	}
	for _, match := range got.Matches {
		if filepath.IsAbs(match.Path) {
			t.Fatalf("match leaked an absolute host path: %+v", match)
		}
	}
}

func TestFileBrowserGrepReturnsExactTotalBeyondSharedExecutorCountCap(t *testing.T) {
	root := t.TempDir()
	for index := range 300 {
		name := fmt.Sprintf("match-%03d.txt", index)
		if err := os.WriteFile(filepath.Join(root, name), []byte("needle\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := (FileBrowser{}).Grep(t.Context(), root, workspaceapp.GrepPlan{Pattern: regexp.MustCompile("needle"), Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 300 || len(got.Matches) != 1 {
		t.Fatalf("Grep = %d retained / total %d, want 1/300", len(got.Matches), got.Total)
	}
}

func TestFileBrowserGrepBoundsRetainedMatchMaterial(t *testing.T) {
	root := t.TempDir()
	line := "needle" + strings.Repeat("x", (1<<20)-len("needle"))
	for index := range 9 {
		name := fmt.Sprintf("large-%02d.txt", index)
		if err := os.WriteFile(filepath.Join(root, name), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := (FileBrowser{}).Grep(t.Context(), root, workspaceapp.GrepPlan{Pattern: regexp.MustCompile("needle"), Limit: 9})
	if err != nil {
		t.Fatal(err)
	}
	material := 0
	for _, match := range got.Matches {
		material += len(match.Path) + len(match.Text)
	}
	if got.Total != 9 || material > 8<<20 || len(got.Matches) >= got.Total {
		t.Fatalf("Grep = %d retained / total %d / %d bytes, want exact total and an 8 MiB whole-row prefix", len(got.Matches), got.Total, material)
	}
}

func TestFileBrowserGrepSearchesOnlyCompleteAdmittedTextFiles(t *testing.T) {
	root := t.TempDir()
	files := map[string][]byte{
		"valid.txt":  []byte("\xef\xbb\xbfneedle\r\n"),
		"binary.txt": []byte("needle\n\x00broken"),
		"long.txt":   []byte("needle" + strings.Repeat("x", workspaceapp.MaxGrepLineBytes)),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oversized, err := os.Create(filepath.Join(root, "oversized.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if truncateErr := oversized.Truncate(workspaceapp.MaxGrepFileBytes + 1); truncateErr != nil {
		t.Fatal(truncateErr)
	}
	if closeErr := oversized.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}

	got, err := (FileBrowser{}).Grep(t.Context(), root, workspaceapp.GrepPlan{
		Pattern: regexp.MustCompile("^needle$"), Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Matches) != 1 || got.Matches[0].Path != "valid.txt" || got.Matches[0].LineNumber != 1 {
		t.Fatalf("Grep = %+v, want only the complete normalized UTF-8 file", got)
	}
}

func TestFileBrowserGrepHonorsPreCanceledContext(t *testing.T) {
	cause := errors.New("workspace search retired")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	_, err := (FileBrowser{}).Grep(ctx, t.TempDir(), workspaceapp.GrepPlan{
		Pattern: regexp.MustCompile("needle"), Limit: 10,
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("Grep error = %v, want cancellation category and owner cause", err)
	}
}

func TestFileBrowserReadPreservesCustomCancellation(t *testing.T) {
	cause := errors.New("workspace read retired")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	_, err := (FileBrowser{}).Read(ctx, t.TempDir(), workspaceapp.FileReadPlan{Path: "file.txt", MaxBytes: workspaceapp.DefaultFileReadBytes})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("Read error = %v, want cancellation category and owner cause", err)
	}
}

func TestGrepFilePreservesReadFailureBeforeTextExclusion(t *testing.T) {
	readFailure := errors.New("search source failed")
	_, err := grepFile(t.Context(), grepFailureReader{content: "needle\n\x00broken\n", err: readFailure}, "file.txt",
		workspaceapp.GrepPlan{Pattern: regexp.MustCompile("needle"), Limit: 10}, 100, 10, 100)
	if !errors.Is(err, readFailure) || !errors.Is(err, textread.ErrInvalidText) {
		t.Fatalf("grep scan error = %v, want read failure and invalid-text diagnosis", err)
	}
}

func TestGrepFilePreservesCancellationDuringInvalidText(t *testing.T) {
	cause := errors.New("workspace search retired during read")
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	readFailure := errors.New("search source failed")
	reader := grepFailureReader{content: "needle\n\x00broken\n", err: readFailure, onRead: func() { cancel(cause) }}
	_, err := grepFile(ctx, reader, "file.txt", workspaceapp.GrepPlan{Pattern: regexp.MustCompile("needle"), Limit: 10}, 100, 10, 100)
	for _, want := range []error{readFailure, textread.ErrInvalidText, context.Canceled, cause} {
		if !errors.Is(err, want) {
			t.Errorf("grep scan error = %v, want retained cause %v", err, want)
		}
	}
}

func TestGrepFilePreservesCapacityAndCancellationCauses(t *testing.T) {
	cause := errors.New("workspace search retired at its source limit")
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	reader := grepFailureReader{content: "needle", err: io.EOF, onRead: func() { cancel(cause) }}
	_, err := grepFile(ctx, reader, "file.txt", workspaceapp.GrepPlan{Pattern: regexp.MustCompile("needle"), Limit: 10}, 5, 10, 100)
	for _, want := range []error{workspaceapp.ErrGrepResultTooLarge, textread.ErrInputTooLarge, context.Canceled, cause} {
		if !errors.Is(err, want) {
			t.Errorf("grep scan error = %v, want retained cause %v", err, want)
		}
	}
}

type grepFailureReader struct {
	content string
	err     error
	onRead  func()
}

func (r grepFailureReader) Read(buffer []byte) (int, error) {
	if r.onRead != nil {
		r.onRead()
	}
	return copy(buffer, r.content), r.err
}

func TestGrepSourcePreservesReadVerificationAndCloseFailures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("needle"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	file, opened, err := filesystem.OpenFileAt(root, "file.txt", workspaceapp.MaxGrepFileBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	search := workspaceGrep{ctx: t.Context(), rootHandle: root,
		plan: workspaceapp.GrepPlan{Pattern: regexp.MustCompile("needle"), Limit: 10}, remainingSource: 100, collectMatches: true}
	err = search.scanSource(grepSource{path: "file.txt", file: file, opened: opened})
	if !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed source error = %v, want closed-file cause", err)
	}
	for _, operation := range []string{"read ", "stat ", "close "} {
		if !strings.Contains(err.Error(), operation+path) {
			t.Errorf("source failure discarded %s: %v", operation, err)
		}
	}
}

func TestRootRelativeFilePathRejectsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(path, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rootRelativeFilePath(root, canonicalRoot, path); err == nil {
		t.Fatal("rootRelativeFilePath accepted a path outside the workspace")
	}
}
