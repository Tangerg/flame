package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWithoutContractOutputWritesOnlyRequestedProjection(t *testing.T) {
	// Enum discovery needs the module directory; artifact writes must be isolated.
	closedVocabulary()
	t.Chdir(t.TempDir())
	projection := filepath.Join(t.TempDir(), "foundation", "protocolValues.generated.ts")
	if err := run("", "", "", "", projection); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(projection)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "export type SessionStatus =") {
		t.Fatal("requested client scalar projection is missing")
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("unrequested artifacts were written to the working directory: %v", entries)
	}
}
