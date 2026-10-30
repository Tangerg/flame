package codeintel

import (
	json "encoding/json/v2"
	"strings"
	"testing"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestDocumentAndResultKeepTheOriginalContentVersion(t *testing.T) {
	source, err := NewDocument(t.TempDir(), "source.go", "package source\n")
	if err != nil {
		t.Fatal(err)
	}
	changed := source
	changed.Content += "changed"
	if err := changed.Validate(); err == nil {
		t.Fatal("altered snapshot retained original version")
	}
	if err := (Transformation{Version: strings.Repeat("0", 64), Content: new("replacement")}).Validate(source); err == nil {
		t.Fatal("another source version accepted")
	}
	if _, err := NewDocument(source.Workspace, "../outside", "text"); err == nil {
		t.Fatal("unconfined path accepted")
	}
	if err := (Transformation{Version: source.Version, Content: new("invalid\x00text")}).Validate(source); err == nil {
		t.Fatal("binary transformation accepted")
	}
}

func TestMaximumEscapedSourceAndBaselineFitTheTransport(t *testing.T) {
	source, err := NewDocument(t.TempDir(), "source.txt", strings.Repeat("\x01", MaxDocumentBytes))
	if err != nil {
		t.Fatal(err)
	}
	input := Inspection{Document: source, Baseline: make([]string, MaxProblems)}
	for i := range input.Baseline {
		input.Baseline[i] = strings.Repeat("a", 64)
	}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(input)
	if err != nil || len(encoded) >= sdk.DefaultMaxLineLength-(1<<20) {
		t.Fatalf("request exceeds transport reserve: %d %v", len(encoded), err)
	}
	if _, err := NewDocument(source.Workspace, source.Path, source.Content+"x"); err == nil {
		t.Fatal("oversized source accepted")
	}
}
