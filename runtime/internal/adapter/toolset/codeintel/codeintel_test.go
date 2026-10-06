package codeintel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnoseMutationReportsUnavailableDiagnosticsInsteadOfACleanResult(t *testing.T) {
	analyzer, err := New(t.Context(), []ServerSpec{{
		Name: "missing", Command: filepath.Join(t.TempDir(), "missing-language-server"),
		LanguageID: "go", Extensions: []string{".go"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	file := filepath.Join(root, "main.go")
	applied := false
	section, err := analyzer.DiagnoseMutation(t.Context(), root, file, func() error {
		applied = true
		return os.WriteFile(file, []byte("package main\n"), 0o600)
	})
	if err != nil || !applied {
		t.Fatalf("DiagnoseMutation = (%q, %v), applied %t", section, err, applied)
	}
	if !strings.HasPrefix(section, "Diagnostics unavailable: ") {
		t.Fatalf("section = %q, want an unavailable report", section)
	}
}
