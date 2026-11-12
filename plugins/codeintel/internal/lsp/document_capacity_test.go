package lsp

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sourcegraph/jsonrpc2"
)

func TestDocumentAdmissionRequiresConfirmedCapacity(t *testing.T) {
	for _, refuseClose := range []bool{false, true} {
		t.Run(fmt.Sprintf("refuse_close=%t", refuseClose), func(t *testing.T) {
			stream := &documentPeer{closed: make(chan struct{}), open: map[string]bool{}}
			c := &client{
				spec:       ServerSpec{Languages: map[string]string{".go": "go"}},
				syncPolicy: documentSynchronization{OpenClose: true, Change: changeFull},
				open:       map[string]openDoc{},
				diags:      map[string]diagSet{},
			}
			c.conn = newConnection(t.Context(), stream, jsonrpc2.AsyncHandler(c))
			t.Cleanup(func() { _ = c.conn.Close() })
			directory := t.TempDir()
			for index := range maxOpenDocuments {
				name := filepath.Join(directory, fmt.Sprintf("file%d.go", index))
				if _, err := c.ensureContent(t.Context(), name, []byte("package p")); err != nil {
					t.Fatal(err)
				}
			}
			failure := errors.New("document close was not delivered")
			if refuseClose {
				stream.closeErr = failure
			}
			name := filepath.Join(directory, "next.go")
			_, err := c.ensureContent(t.Context(), name, []byte("package next"))
			if refuseClose {
				if !errors.Is(err, failure) {
					t.Fatalf("admission error = %v, want %v", err, failure)
				}
				if stream.open[pathToURI(name)] {
					t.Fatal("new document opened before capacity was released")
				}
				stream.closeErr = nil
				if _, err := c.ensureContent(t.Context(), name, []byte("package next")); err != nil {
					t.Fatalf("admission after confirmed release: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if stream.maximumOpen > maxOpenDocuments || !stream.open[pathToURI(name)] {
				t.Fatalf("remote peak = %d; new document open = %t", stream.maximumOpen, stream.open[pathToURI(name)])
			}
		})
	}
}

type documentPeer struct {
	closed      chan struct{}
	closeOnce   sync.Once
	closeErr    error
	open        map[string]bool
	maximumOpen int
}

func (s *documentPeer) ReadObject(any) error {
	<-s.closed
	return io.EOF
}

func (s *documentPeer) WriteObject(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var notification struct {
		Method string `json:"method"`
		Params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		} `json:"params"`
	}
	if err := json.Unmarshal(data, &notification); err != nil {
		return err
	}
	uri := notification.Params.TextDocument.URI
	switch notification.Method {
	case "textDocument/didOpen":
		s.open[uri] = true
		s.maximumOpen = max(s.maximumOpen, len(s.open))
	case "textDocument/didClose":
		if s.closeErr != nil {
			return s.closeErr
		}
		delete(s.open, uri)
	}
	return nil
}

func (s *documentPeer) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}
