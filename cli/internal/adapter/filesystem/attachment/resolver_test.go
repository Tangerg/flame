package attachment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func TestResolveClassifiesAndCanonicalizesWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "notes.md")
	writeFixture(t, path, "# notes\n")
	resolver := newFixtureResolver(t, root)
	got := resolveFixture(t, resolver, "docs/notes.md")
	canonical, _ := filepath.EvalSymlinks(path)
	requireTextAttachment(t, got, canonical)
	again := resolveFixture(t, resolver, path)
	if again.ID != got.ID {
		t.Fatalf("stable identity = %s, want %s", again.ID, got.ID)
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newFixtureResolver(t *testing.T, root string) *Resolver {
	t.Helper()
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func resolveFixture(t *testing.T, resolver *Resolver, path string) agent.Attachment {
	t.Helper()
	attachment, err := resolver.Resolve(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	return attachment
}

func requireTextAttachment(t *testing.T, got agent.Attachment, canonical string) {
	t.Helper()
	if got.ID == "" || got.Kind != protocol.ContentBlockText || got.Name != "docs/notes.md" || got.Path != canonical || got.MimeType != "text/markdown" || got.Size != 8 {
		t.Fatalf("attachment = %+v", got)
	}
}

func TestResolveRejectsDirectoriesAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	resolver, _ := New(root)
	if _, err := resolver.Resolve(t.Context(), root); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("directory error = %v", err)
	}
	resolver.maxBytes = 2
	path := filepath.Join(root, "large.txt")
	if err := os.WriteFile(path, []byte("large"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(t.Context(), path); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize error = %v", err)
	}
}

func TestResolveRejectsContentTheRuntimeProtocolCannotRepresent(t *testing.T) {
	root := t.TempDir()
	resolver, _ := New(root)
	path := filepath.Join(root, "archive.bin")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(t.Context(), path); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("binary attachment error = %v", err)
	}
}

func TestCompleteRanksFilesAndSkipsDependencyInternals(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"internal/cache/store.go", "cache_test.go", ".git/cache-secret", "node_modules/cache.js"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resolver, _ := New(root)
	got, err := resolver.Complete(t.Context(), "cache")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Path != "cache_test.go" {
		t.Fatalf("matches = %+v", got)
	}
	for _, match := range got {
		if strings.Contains(match.Path, ".git") || strings.Contains(match.Path, "node_modules") {
			t.Fatalf("ignored path returned: %+v", match)
		}
	}
}

func TestCompleteHonorsCanceledContext(t *testing.T) {
	resolver, _ := New(t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := resolver.Complete(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestCompleteOwnsOneFiniteProductResultBudget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for index := range completionResultLimit + 10 {
		name := filepath.Join(root, fmt.Sprintf("match-%03d.txt", index))
		if err := os.WriteFile(name, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	resolver, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	matches, err := resolver.Complete(t.Context(), "match")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != completionResultLimit {
		t.Fatalf("completion returned %d matches, want %d", len(matches), completionResultLimit)
	}
}

func TestAttachmentIdentityPreservesFieldBoundaries(t *testing.T) {
	left := (attachmentIdentity{canonicalPath: "a", size: 12, modifiedAt: 3}).digest()
	right := (attachmentIdentity{canonicalPath: "a1", size: 2, modifiedAt: 3}).digest()
	if left == right {
		t.Fatal("different attachment identity fields produced the same digest")
	}
}
