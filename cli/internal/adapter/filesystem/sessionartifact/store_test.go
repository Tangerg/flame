package sessionartifact

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/agent/session"
)

func TestStorePublishesWithoutClobberingAndLoadsPortableJSON(t *testing.T) {
	workspace := t.TempDir()
	document, err := session.NewDocument(protocol.ExportFormatJSON, []byte(`{"version":17}`))
	if err != nil {
		t.Fatal(err)
	}
	store := Store{}
	first, err := store.Publish(workspace, "Portable session", "archive.json", document)
	if err != nil {
		t.Fatal(err)
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if first != filepath.Join(canonicalWorkspace, "archive.json") {
		t.Fatalf("first path = %q", first)
	}
	published, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(published, document.Bytes()) {
		t.Fatalf("published bytes = %q, want exact document %q", published, document.Bytes())
	}
	if writeFileErr := os.WriteFile(first, []byte("different"), 0o600); writeFileErr != nil {
		t.Fatal(writeFileErr)
	}
	second, err := store.Publish(workspace, "Portable session", "archive.json", document)
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Fatal("different existing document was overwritten")
	}
	loaded, err := store.Load(workspace, filepath.Base(second))
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Bytes()) != `{"version":17}` {
		t.Fatalf("loaded body = %q", loaded.Bytes())
	}
}

func TestStoreRejectsPathsAsExportNames(t *testing.T) {
	document, err := session.NewDocument(protocol.ExportFormatMarkdown, []byte("# Session"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{}).Publish(t.TempDir(), "Session", "../escape.md", document); err == nil {
		t.Fatal("path-shaped export name was accepted")
	}
}

func TestStoreRejectsNonRegularImports(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "artifact.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	_, err := (Store{}).Load(workspace, link)
	if err == nil || err.Error() != "session artifact is not a regular file" {
		t.Fatalf("Load(%q) error = %v, want non-regular artifact", link, err)
	}
}
