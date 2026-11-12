package lsp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/sourcegraph/jsonrpc2"
)

func TestEnsureOpenRejectsOversizedDocumentBeforeNotification(t *testing.T) {
	const app2DocumentLimit = 8 << 20

	content := bytes.Repeat([]byte("x"), app2DocumentLimit+1)
	path := filepath.Join(t.TempDir(), "generated.go")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	uri := pathToURI(path)
	client := &client{workspace: testWorkspace(t, filepath.Dir(path)),
		spec: ServerSpec{Languages: map[string]string{".go": "go"}},
		open: map[string]openDoc{
			uri: {version: 1, hash: sha256.Sum256(content)},
		},
	}

	if version, err := client.ensureOpen(t.Context(), path); !errors.Is(err, errDocumentTooLarge) || !errors.Is(err, filesystem.ErrFileTooLarge) {
		t.Fatalf("ensureOpen = (%d, %v), want ErrDocumentTooLarge", version, err)
	}
	if client.open[uri].version != 1 {
		t.Fatalf("rejected sync changed open document state: %+v", client.open[uri])
	}
}

func TestReadDocumentHonorsExactBoundaryAndCancellation(t *testing.T) {
	content := bytes.Repeat([]byte("x"), int(maxDocumentBytes))
	path := filepath.Join(t.TempDir(), "boundary.go")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	read, err := readDocument(t.Context(), testWorkspace(t, filepath.Dir(path)), path)
	if err != nil {
		t.Fatalf("read exact boundary: %v", err)
	}
	if len(read) != len(content) {
		t.Fatalf("read %d bytes, want %d", len(read), len(content))
	}

	canceled, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("stop document read")
	cancel(cause)
	if _, err := readDocument(canceled, nil, path); !errors.Is(err, cause) {
		t.Fatalf("canceled read error = %v, want %v", err, cause)
	}
}

func TestReadDocumentRejectsUnsupportedSources(t *testing.T) {
	directory := t.TempDir()
	invalid := filepath.Join(directory, "invalid.go")
	if err := os.WriteFile(invalid, []byte{'p', 'a', 'c', 'k', 'a', 'g', 'e', 0}, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{directory, invalid} {
		if _, err := readDocument(t.Context(), testWorkspace(t, filepath.Dir(path)), path); !errors.Is(err, errUnsupportedDocument) {
			t.Fatalf("readDocument(%q) error = %v, want ErrUnsupportedDocument", path, err)
		}
	}
}

// TestEnsureOpenBoundsTheSynchronizedDocumentSet pins the resource limit on a
// client that lives as long as the runtime: the least recently synced document
// is closed rather than accumulated, and closing it releases the server's copy
// too. Diagnostics for a document this client no longer synchronizes are
// dropped, so the two maps stay bounded together.
func TestEnsureOpenBoundsTheSynchronizedDocumentSet(t *testing.T) {
	agent, server := net.Pipe()
	t.Cleanup(func() { _ = agent.Close(); _ = server.Close() })

	closed := make(chan string, maxOpenDocuments)
	go func() {
		reader := bufio.NewReader(server)
		for {
			var frame struct {
				Method string `json:"method"`
				Params struct {
					TextDocument struct {
						URI string `json:"uri"`
					} `json:"textDocument"`
				} `json:"params"`
			}
			if err := (lspObjectCodec{}).ReadObject(reader, &frame); err != nil {
				return
			}
			if frame.Method == "textDocument/didClose" {
				closed <- frame.Params.TextDocument.URI
			}
		}
	}()

	c := &client{
		spec:    ServerSpec{Languages: map[string]string{".go": "go"}},
		open:    map[string]openDoc{},
		diags:   map[string]diagSet{},
		updated: make(chan struct{}),
	}
	c.conn = newConnection(
		t.Context(), jsonrpc2.NewBufferedStream(agent, lspObjectCodec{}), jsonrpc2.AsyncHandler(c),
	)
	t.Cleanup(func() { _ = c.conn.Close() })

	directory := t.TempDir()
	c.workspace = testWorkspace(t, directory)
	c.syncPolicy = documentSynchronization{OpenClose: true, Change: changeFull}
	var first string
	for index := range maxOpenDocuments + 1 {
		path := filepath.Join(directory, fmt.Sprintf("file%03d.go", index))
		if err := os.WriteFile(path, fmt.Appendf(nil, "package p // %d\n", index), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := c.ensureOpen(t.Context(), path); err != nil {
			t.Fatalf("ensureOpen %s: %v", path, err)
		}
		if index == 0 {
			first = pathToURI(path)
		}
	}

	select {
	case evicted := <-closed:
		if evicted != first {
			t.Fatalf("closed %q, want the least recently synced %q", evicted, first)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the synchronized set grew past its bound without closing anything")
	}

	c.mu.Lock()
	open := len(c.open)
	_, stillOpen := c.open[first]
	c.mu.Unlock()
	if open > maxOpenDocuments || stillOpen {
		t.Fatalf("open documents = %d (evicted still present = %v)", open, stillOpen)
	}

	c.storeDiagnostics(publishDiagnosticsParams{URI: first})
	c.mu.Lock()
	_, kept := c.diags[first]
	c.mu.Unlock()
	if kept {
		t.Fatal("kept diagnostics for a document the client no longer synchronizes")
	}
	version, err := c.ensureContent(t.Context(), uriToPath(first), []byte("package different"))
	if err != nil || version <= maxOpenDocuments+1 {
		t.Fatalf("version after eviction %d, %v", version, err)
	}
	c.storeDiagnostics(publishDiagnosticsParams{URI: first, Version: 1, Diagnostics: []Diagnostic{}})
	if _, err := c.diagnosticsVersion(t.Context(), uriToPath(first), version, 0); !errors.As(err, new(*DiagnosticsPendingError)) {
		t.Fatalf("stale diagnostics served: %v", err)
	}
}

func testWorkspace(t *testing.T, directory string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root
}

func TestWorkspaceProhibitsExternalLSPDocuments(t *testing.T) {
	workspace := testWorkspace(t, t.TempDir())
	outside := filepath.Join(t.TempDir(), "external.go")
	if err := os.WriteFile(outside, []byte("package private"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDocument(t.Context(), workspace, outside); err == nil {
		t.Fatal("read external file")
	}
	link := filepath.Join(workspace.Name(), "alias.go")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}
	if _, err := readDocument(t.Context(), workspace, link); err == nil {
		t.Fatal("opened external symlink")
	}
	parent := t.TempDir()
	directory := filepath.Join(parent, "project")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	pinned := testWorkspace(t, directory)
	name := filepath.Join(directory, "external.go")
	if err := os.WriteFile(name, []byte("package original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(directory, filepath.Join(parent, "original")); err != nil {
		t.Skip(err)
	}
	if err := os.Symlink(filepath.Dir(outside), directory); err != nil {
		t.Skip(err)
	}
	got, err := readDocument(t.Context(), pinned, name)
	if err != nil || string(got) != "package original" {
		t.Fatalf("directory replacement changed authority: %q %v", got, err)
	}
}

func TestNegotiatedDocumentPolicies(t *testing.T) {
	for raw, want := range map[string]documentSynchronization{
		"0": {}, "1": {OpenClose: true, Change: changeFull}, "2": {OpenClose: true, Change: changeIncremental},
		`{"openClose":false,"change":0}`:                            {},
		`{"openClose":true,"change":2,"save":{"includeText":true}}`: {OpenClose: true, Change: changeIncremental, Save: saveSynchronization{Enabled: true, IncludeText: true}},
	} {
		got, err := negotiateDocumentSync([]byte(raw))
		if err != nil || got != want {
			t.Fatalf("%s: %+v, %v", raw, got, err)
		}
	}
	if got := documentEnd([]byte("a\r\nb\r😀!")); got != (Position{Line: 2, Character: 3}) {
		t.Fatal(got)
	}
	table, err := newServerTable(DefaultServers())
	if err != nil {
		t.Fatal(err)
	}
	tsx, _ := table.forFile("view.tsx")
	if tsx.Languages[".tsx"] != "typescriptreact" || tsx.Languages[".ts"] != "typescript" {
		t.Fatal(tsx)
	}
}
