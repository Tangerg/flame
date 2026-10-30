package lsp

import (
	"bufio"
	"errors"
	"io"
	"net"
	"path/filepath"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestDiagnosticsRequireTheExactPublishedVersion(t *testing.T) {
	for _, published := range []int{0, 1, 2, 3} {
		agent, server := net.Pipe()
		t.Cleanup(func() { _ = agent.Close(); _ = server.Close() })
		go func() { _, _ = io.Copy(io.Discard, server) }()
		path := filepath.Join(t.TempDir(), "source.go")
		uri := pathToURI(path)
		c := &client{open: map[string]openDoc{uri: {version: 2}}, diags: map[string]diagSet{uri: {version: published, diagnostics: []Diagnostic{{Message: "original problem"}}}}, updated: make(chan struct{})}
		c.conn = jsonrpc2.NewConn(t.Context(), jsonrpc2.NewBufferedStream(agent, lspObjectCodec{}), jsonrpc2.AsyncHandler(c))
		t.Cleanup(func() { _ = c.conn.Close() })
		result, err := c.diagnosticsVersion(t.Context(), path, 2, 0)
		if published == 2 {
			if err != nil || len(result) != 1 {
				t.Fatalf("exact diagnostics: %+v %v", result, err)
			}
		} else if !errors.As(err, new(*DiagnosticsPendingError)) || result != nil {
			t.Fatalf("version %d accepted for version 2: %+v %v", published, result, err)
		}
	}
}

func TestSnapshotSynchronizationDoesNotReadThePath(t *testing.T) {
	agent, server := net.Pipe()
	t.Cleanup(func() { _ = agent.Close(); _ = server.Close() })
	content := make(chan string, 1)
	go func() {
		var frame struct {
			Params didOpenParams `json:"params"`
		}
		if err := (lspObjectCodec{}).ReadObject(bufio.NewReader(server), &frame); err == nil {
			content <- frame.Params.TextDocument.Text
		}
	}()
	c := &client{spec: ServerSpec{LanguageID: "go"}, open: map[string]openDoc{}, diags: map[string]diagSet{}, updated: make(chan struct{})}
	c.conn = jsonrpc2.NewConn(t.Context(), jsonrpc2.NewBufferedStream(agent, lspObjectCodec{}), jsonrpc2.AsyncHandler(c))
	t.Cleanup(func() { _ = c.conn.Close() })
	if _, err := c.ensureContent(t.Context(), filepath.Join(t.TempDir(), "does-not-exist.go"), []byte("captured source")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-content:
		if got != "captured source" {
			t.Fatalf("synchronized %q", got)
		}
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
}
