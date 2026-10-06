package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewFilesRequiresCompleteDependencies(t *testing.T) {
	scope := newScope(t, "", "", fileReadPaths{})
	for _, test := range []struct {
		name  string
		scope *Scope
		files FileBrowser
	}{
		{name: "scope", files: &fileReadPort{}},
		{name: "browser", scope: scope},
		{name: "typed nil browser", scope: scope, files: (*fileReadPort)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if files, err := NewFiles(test.scope, test.files); err == nil || files != nil {
				t.Fatalf("NewFiles = (%v, %v), want incomplete construction rejected", files, err)
			}
		})
	}
}

func newFiles(t *testing.T, scope *Scope, browser FileBrowser) *Files {
	t.Helper()
	files, err := NewFiles(scope, browser)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

type fileReadPort struct {
	input      FileReadPlan
	result     FileReadResult
	grepInput  GrepPlan
	grepResult GrepResult
	grepCalled bool
}

func (f *fileReadPort) List(context.Context, string, FileListOptions) ([]FileEntry, error) {
	return nil, nil
}

func (f *fileReadPort) Read(_ context.Context, _ string, input FileReadPlan) (FileReadResult, error) {
	f.input = input
	return f.result, nil
}

func (f *fileReadPort) Grep(_ context.Context, _ string, input GrepPlan) (GrepResult, error) {
	f.grepCalled = true
	f.grepInput = input
	return f.grepResult, nil
}

type fileReadPaths struct{}

func (fileReadPaths) ResolveExistingDir(path string) (string, error) { return path, nil }
func (fileReadPaths) ResolveInRoot(_ string, path string) (string, error) {
	return path, nil
}
func (fileReadPaths) ResolveExistingInRoot(_ string, path string) (string, error) {
	return path, nil
}

func TestFilesReadNormalizesBudgetBeforeCallingPort(t *testing.T) {
	port := &fileReadPort{result: FileReadResult{Content: "text", TotalLines: 1, EndLine: 1}}
	files := newFiles(t, newScope(t, t.TempDir(), "", fileReadPaths{}), port)

	got, err := files.Read(t.Context(), "", FileReadInput{Path: "file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "text" || port.input.MaxBytes != DefaultFileReadBytes {
		t.Fatalf("Read = %+v, port budget = %d", got, port.input.MaxBytes)
	}
}

func TestFilesHeadRejectsByteTruncatedPortResult(t *testing.T) {
	port := &fileReadPort{result: FileReadResult{
		Content: "prefix", TotalLines: 2, EndLine: 1, Truncated: true, OutputTruncated: true,
	}}
	files := newFiles(t, newScope(t, t.TempDir(), "", fileReadPaths{}), port)

	_, err := files.Head(t.Context(), "", "file.txt", mustHeadLineLimit(t, 1))
	if !errors.Is(err, ErrFileReadTooLarge) {
		t.Fatalf("Head error = %v, want ErrFileReadTooLarge", err)
	}
}

func TestFilesGrepOwnsQueryLimitAndPortResultEnvelope(t *testing.T) {
	t.Run("invalid regex", func(t *testing.T) {
		port := &fileReadPort{}
		files := newFiles(t, newScope(t, t.TempDir(), "", fileReadPaths{}), port)

		if _, err := files.Grep(t.Context(), "", GrepInput{Query: "["}); err == nil {
			t.Fatal("Grep accepted an invalid regular expression")
		}
		if port.grepCalled {
			t.Fatal("Grep called the filesystem port before validating the query")
		}
	})

	t.Run("oversized query", func(t *testing.T) {
		port := &fileReadPort{}
		files := newFiles(t, newScope(t, t.TempDir(), "", fileReadPaths{}), port)

		if _, err := files.Grep(t.Context(), "", GrepInput{Query: strings.Repeat("x", (64<<10)+1)}); err == nil {
			t.Fatal("Grep accepted a query larger than 64 KiB")
		}
		if port.grepCalled {
			t.Fatal("Grep called the filesystem port with an oversized query")
		}
	})

	t.Run("caller limit", func(t *testing.T) {
		port := &fileReadPort{}
		files := newFiles(t, newScope(t, t.TempDir(), "", fileReadPaths{}), port)

		if _, err := files.Grep(t.Context(), "", GrepInput{Query: "needle", Limit: mustGrepResultLimit(t, 100_000)}); err != nil {
			t.Fatal(err)
		}
		if port.grepInput.Limit != 1000 {
			t.Fatalf("port limit = %d, want Application-owned maximum 1000", port.grepInput.Limit)
		}
	})

}

func mustHeadLineLimit(t *testing.T, lines int) HeadLineLimit {
	t.Helper()
	limit, err := NewHeadLineLimit(lines)
	if err != nil {
		t.Fatalf("NewHeadLineLimit(%d): %v", lines, err)
	}
	return limit
}

func mustGrepResultLimit(t *testing.T, matches int) GrepResultLimit {
	t.Helper()
	limit, err := NewGrepResultLimit(matches)
	if err != nil {
		t.Fatalf("NewGrepResultLimit(%d): %v", matches, err)
	}
	return limit
}
