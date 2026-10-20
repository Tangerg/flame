package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPublishedVersionKeepsItsMetadataWhenAnotherModuleChanges(t *testing.T) {
	name := filepath.Join(t.TempDir(), "candidate.info")
	first, err := versionInfo(name, "v0.1.0-candidate.1", "2026-10-20T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if err := publishImmutable(name, first); err != nil {
		t.Fatal(err)
	}
	repeated, err := versionInfo(name, "v0.1.0-candidate.1", "2026-10-21T00:00:00Z")
	if err != nil || !bytes.Equal(first, repeated) {
		t.Fatalf("version metadata changed: %s, %v", repeated, err)
	}
	if err := publishImmutable(name, repeated); err != nil {
		t.Fatal(err)
	}
}

func TestVersionPublicationRefusesInvalidExistingMetadata(t *testing.T) {
	for _, content := range []string{`{`, `{"Version":"v0.1.0-other","Time":"2026-10-20T00:00:00Z"}`, `{"Version":"v0.1.0-candidate.1","Time":"invalid"}`} {
		name := filepath.Join(t.TempDir(), "candidate.info")
		if err := os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := versionInfo(name, "v0.1.0-candidate.1", "2026-10-21T00:00:00Z"); err == nil {
			t.Fatal("invalid published metadata was accepted")
		}
		stored, err := os.ReadFile(name)
		if err != nil || string(stored) != content {
			t.Fatalf("invalid evidence was overwritten: %q, %v", stored, err)
		}
	}
}

func TestVersionPublicationCannotReplaceAnExistingArtifact(t *testing.T) {
	name := filepath.Join(t.TempDir(), "candidate.mod")
	if err := publishImmutable(name, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := publishImmutable(name, []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := publishImmutable(name, []byte("different")); err == nil {
		t.Fatal("reused version changed content")
	}
	actual, err := os.ReadFile(name)
	if err != nil || string(actual) != "original" {
		t.Fatalf("published bytes = %q, %v", actual, err)
	}
}

func TestConcurrentVersionPublicationHasOneContentOwner(t *testing.T) {
	name := filepath.Join(t.TempDir(), "candidate.zip")
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for _, content := range []string{"first", "second"} {
		wg.Go(func() { errors <- publishImmutable(name, []byte(content)) })
	}
	wg.Wait()
	close(errors)
	succeeded := 0
	for err := range errors {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("publishers accepted = %d, want one", succeeded)
	}
	actual, err := os.ReadFile(name)
	if err != nil || (string(actual) != "first" && string(actual) != "second") {
		t.Fatalf("partial artifact = %q, %v", actual, err)
	}
}
