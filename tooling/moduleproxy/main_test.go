package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

func TestWorktreeArchivesTheSameFrozenModuleAsItsPrimaryCheckout(t *testing.T) {
	repository := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.CommandContext(t.Context(), "git", args...)
		command.Dir = repository
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "--quiet")
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module example.com/owner\n\ngo 1.27.3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "go.mod")
	git("-c", "user.name=Archive Test", "-c", "user.email=archive@example.invalid", "commit", "--quiet", "-m", "frozen source")
	commit := git("rev-parse", "HEAD")
	worktree := filepath.Join(t.TempDir(), "linked source")
	git("worktree", "add", "--detach", worktree, "HEAD")
	if err := os.WriteFile(filepath.Join(repository, "later.txt"), []byte("another checkout's later commit"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "later.txt")
	git("-c", "user.name=Archive Test", "-c", "user.email=archive@example.invalid", "commit", "--quiet", "-m", "later source")
	version := module.Version{Path: "example.com/owner", Version: "v0.1.0-test.1"}
	var expected, actual bytes.Buffer
	if err := modzip.CreateFromVCS(&expected, version, repository, commit, ""); err != nil {
		t.Fatal(err)
	}
	frozen, err := archiveModule(&actual, version, worktree, "HEAD", "")
	if err != nil {
		t.Fatal(err)
	}
	if frozen != commit {
		t.Fatal("worktree archive changed its original source commit")
	}
	if !bytes.Equal(expected.Bytes(), actual.Bytes()) {
		t.Fatal("worktree changed the frozen standard module archive")
	}
}

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
