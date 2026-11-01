package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestObservationCannotRunWorkspaceGitHelpers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("*.txt filter=fixture=typed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "fixture")
	helper := filepath.Join(dir, ".git", "fixture")
	marker := filepath.Join(dir, ".git", "executed")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\ntouch '"+marker+"'\ncat\n"), 0700); err != nil {
		t.Fatal(err)
	}
	git("config", "core.fsmonitor", helper)
	git("config", "filter.fixture=typed.clean", helper)
	git("config", "filter.fixture=typed.required", "true")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"status", "--porcelain=v1", "--ignore-submodules=dirty"}, {"diff", "--numstat", "--no-textconv", "--no-ext-diff", "HEAD"}} {
		r, err := At(t.Context(), dir, args...)
		if err != nil || r.ExitCode != 0 || len(r.Stdout) == 0 {
			t.Fatalf("git %v: %+v, %v", args, r, err)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, ".git", "info", "attributes"), []byte("*.txt -diff filter=fixture=typed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	infoDiff, err := At(t.Context(), dir, "diff", "--numstat", "--no-ext-diff", "--no-textconv", "HEAD")
	if err != nil || string(infoDiff.Stdout) != "-\t-\ta.txt\n" {
		t.Fatalf("metadata attributes differ: %+v, %v", infoDiff, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("workspace helper executed", err)
	}
}

func TestReadViewKeepsLinkedWorktreeSplitIndexAndSHA256(t *testing.T) {
	root := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	main := filepath.Join(root, "main")
	git(root, "init", "-q", "--object-format=sha256", main)
	if err := os.WriteFile(filepath.Join(main, "tracked"), []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(main, "add", "tracked")
	git(main, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "first")
	work := filepath.Join(root, "linked")
	git(main, "worktree", "add", "-q", "--detach", work, "HEAD")
	git(work, "update-index", "--split-index")
	if err := os.WriteFile(filepath.Join(work, "tracked"), []byte("two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := At(t.Context(), work, "diff", "--numstat", "--no-textconv", "--no-ext-diff", "HEAD")
	if err != nil || result.ExitCode != 0 || string(result.Stdout) != "1\t1\ttracked\n" {
		t.Fatalf("linked diff=%+v, %v", result, err)
	}
}
