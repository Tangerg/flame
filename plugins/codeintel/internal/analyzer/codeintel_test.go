package analyzer

import (
	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
	"github.com/Tangerg/flame/plugins/codeintel/internal/lsp"
	"path/filepath"
	"testing"
)

func TestUnavailableLanguageServerDoesNotProduceACleanReport(t *testing.T) {
	analysis, err := New(t.Context(), []lsp.ServerSpec{{Name: "missing", Command: filepath.Join(t.TempDir(), "missing-server"), LanguageID: "go", Extensions: []string{".go"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := analysis.Close(); err != nil {
			t.Error(err)
		}
	})
	source, err := codeintel.NewDocument(t.TempDir(), "main.go", "package main\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := analysis.Inspect(t.Context(), codeintel.Inspection{Document: source}); err == nil {
		t.Fatal("unavailable diagnostics were reported as clean")
	}
}
