package session

import (
	"bytes"
	"testing"
)

func TestDocumentOwnsAndValidatesPortableContent(t *testing.T) {
	body := []byte(`{"version":17}`)
	document, err := NewDocument(JSONFormat, body)
	if err != nil {
		t.Fatal(err)
	}
	body[0] = 'x'
	copy := document.Bytes()
	copy[0] = 'x'
	if got := string(document.Bytes()); got != `{"version":17}` {
		t.Fatalf("document body = %q", got)
	}
	if !document.Importable() {
		t.Fatal("valid JSON document is not importable")
	}
	markdown, err := NewDocument(MarkdownFormat, []byte("# Session"))
	if err != nil {
		t.Fatal(err)
	}
	if markdown.Importable() {
		t.Fatal("Markdown document is importable")
	}
}

func TestDocumentRejectsOversizedPortableContent(t *testing.T) {
	body := bytes.Repeat([]byte("x"), MaximumDocumentBytes+1)
	if _, err := NewDocument(MarkdownFormat, body); err == nil {
		t.Fatal("oversized session document was accepted")
	}
}
