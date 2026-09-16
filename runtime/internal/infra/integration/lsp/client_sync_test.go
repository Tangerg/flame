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
	client := &client{
		spec: ServerSpec{LanguageID: "go"},
		open: map[string]openDoc{
			uri: {version: 1, hash: sha256.Sum256(content)},
		},
	}

	if version, err := client.ensureOpen(t.Context(), path); !errors.Is(err, ErrDocumentTooLarge) {
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

	read, err := readDocument(t.Context(), path)
	if err != nil {
		t.Fatalf("read exact boundary: %v", err)
	}
	if len(read) != len(content) {
		t.Fatalf("read %d bytes, want %d", len(read), len(content))
	}

	canceled, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("stop document read")
	cancel(cause)
	if _, err := readDocument(canceled, path); !errors.Is(err, cause) {
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
		if _, err := readDocument(t.Context(), path); !errors.Is(err, ErrUnsupportedDocument) {
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
		spec:    ServerSpec{LanguageID: "go"},
		open:    map[string]openDoc{},
		diags:   map[string]diagSet{},
		updated: make(chan struct{}),
	}
	c.conn = jsonrpc2.NewConn(
		t.Context(), jsonrpc2.NewBufferedStream(agent, lspObjectCodec{}), jsonrpc2.AsyncHandler(c),
	)
	t.Cleanup(func() { _ = c.conn.Close() })

	directory := t.TempDir()
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
}
