package main

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestMarkdownRendersVisibleSessionEvidence(t *testing.T) {
	doc := protocol.ArtifactSession{ID: "ses_a", Title: "Doc", Workspace: protocol.WorkspaceRef{Path: "/project"}, Model: "captured"}
	got, err := renderSessionMarkdown(doc, []protocol.ArtifactItem{
		{Type: protocol.ItemTypeUserMessage, Content: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "explain this"}}},
		{Type: protocol.ItemTypeReasoning, Text: "redacted canary", Redacted: true},
		{Type: protocol.ItemTypeCompaction, Summary: "Earlier constraints", DroppedMessages: 7},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Doc", "/project", "captured", "## User", "explain this", "## Context compacted", "dropped messages: 7", "Earlier constraints"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "redacted canary") {
		t.Fatal("private reasoning rendered")
	}
}

func TestRendererRejectsTruncatedOrChangedSnapshotRanges(t *testing.T) {
	src := []byte(`{}`)
	identity := plugin.IdentifyDocument(src)
	for _, payload := range []string{"eA==", "eHg="} {
		_, err := readSessionSnapshot(t.Context(), identity, func(r plugin.DocumentRange) (string, error) { return payload, nil })
		if err == nil {
			t.Fatal("accepted a truncated or substituted range")
		}
	}
}

func TestEmptySessionMarkdownKeepsTheSessionIdentity(t *testing.T) {
	if got, err := renderSessionMarkdown(protocol.ArtifactSession{ID: "ses_empty"}, nil, nil); err != nil || got != "# ses_empty\n" {
		t.Fatalf("empty document %q, %v", got, err)
	}
}
