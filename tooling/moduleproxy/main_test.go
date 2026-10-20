package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

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
