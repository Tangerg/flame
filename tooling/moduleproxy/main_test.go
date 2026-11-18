package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	first, err := versionInfo(name, "v0.1.0-candidate.1", "2026-10-20T00:00:00Z", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishImmutable(name, first); err != nil {
		t.Fatal(err)
	}
	repeated, err := versionInfo(name, "v0.1.0-candidate.1", "2026-10-21T00:00:00Z", true)
	if err != nil || !bytes.Equal(first, repeated) {
		t.Fatalf("version metadata changed: %s, %v", repeated, err)
	}
	if err := publishImmutable(name, repeated); err != nil {
		t.Fatal(err)
	}
	if _, err := versionInfo(name, "v0.1.0-candidate.1", "2026-10-21T00:00:00Z", false); err == nil {
		t.Fatal("a tree publication accepted a different time for the same version")
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
