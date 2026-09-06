package git

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaseDiffUsesExactBranchReferences(t *testing.T) {
	for _, source := range []string{"main", "master", "origin/trunk"} {
		t.Run(source, func(t *testing.T) {
			dir := initRepo(t)
			switch source {
			case "master":
				gitTestCommand(t, dir, "branch", "-m", "main", "master")
			case "origin/trunk":
				gitTestCommand(t, dir, "update-ref", "refs/remotes/origin/trunk", "HEAD")
				gitTestCommand(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
			}
			gitTestCommand(t, dir, "checkout", "-b", "feature")
			write(t, dir, "a.txt", "feature change\n")
			gitTestCommand(t, dir, "commit", "-am", "feature")
			// Short revision names let a tag or local branch shadow the selected
			// base and can turn the committed feature diff into an empty result.
			gitTestCommand(t, dir, "tag", source)
			if source == "origin/trunk" {
				gitTestCommand(t, dir, "branch", source)
			}
			if source == "main" {
				gitTestCommand(t, dir, "checkout", "--detach")
			}
			patch, err := RawDiff(t.Context(), dir, "", Base, testMaxDiffBytes)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(patch, "+feature change") {
				t.Fatalf("base diff used a shadowing ref: %q", patch)
			}
			files, err := testDiff(t.Context(), dir, "", Base)
			if err != nil || len(files) != 1 || files[0].Path != "a.txt" {
				t.Fatalf("structured base diff = %+v, %v", files, err)
			}
		})
	}
}

func TestBaseDiffDistinguishesMissingBaseFromBrokenObjects(t *testing.T) {
	dir := initRepo(t)
	gitTestCommand(t, dir, "checkout", "-b", "feature")
	head := strings.TrimSpace(gitTestCommandOutput(t, dir, "rev-parse", "HEAD"))
	write(t, dir, filepath.Join(".git", "refs", "heads", "main"), strings.Repeat("1", len(head))+"\n")
	if _, err := RawDiff(t.Context(), dir, "", Base, testMaxDiffBytes); err == nil || errors.Is(err, ErrNoBase) {
		t.Fatalf("broken base object error = %v, want the Git failure", err)
	}
	if _, err := testDiff(t.Context(), dir, "", Base); err == nil || errors.Is(err, ErrNoBase) {
		t.Fatalf("structured broken base object error = %v, want the Git failure", err)
	}
}

func TestBaseQueriesPreserveCancellationAndConfigurationFailures(t *testing.T) {
	for _, query := range []struct {
		name string
		read func(context.Context, string) (string, error)
	}{
		{name: "default branch", read: defaultBranchTip},
		{name: "merge base", read: mergeBase},
	} {
		t.Run(query.name, func(t *testing.T) {
			dir := initRepo(t)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := query.read(ctx, dir); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled query error = %v", err)
			}
			write(t, dir, filepath.Join(".git", "config"), "[")
			if _, err := query.read(t.Context(), dir); err == nil || errors.Is(err, ErrNoBase) {
				t.Fatalf("configuration error = %v, want the Git failure", err)
			}
		})
	}
}

func TestBaseDiffKeepsExpectedMissingBaseOutcomes(t *testing.T) {
	for _, scenario := range []string{"no default branch", "unborn head", "unrelated history", "missing remote default"} {
		t.Run(scenario, func(t *testing.T) {
			dir := initRepo(t)
			switch scenario {
			case "no default branch":
				gitTestCommand(t, dir, "branch", "-m", "main", "topic")
			case "unborn head":
				gitTestCommand(t, dir, "checkout", "--orphan", "topic")
			case "unrelated history":
				gitTestCommand(t, dir, "checkout", "--orphan", "topic")
				gitTestCommand(t, dir, "commit", "-m", "unrelated")
			case "missing remote default":
				gitTestCommand(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/missing")
			}
			if _, err := RawDiff(t.Context(), dir, "", Base, testMaxDiffBytes); !errors.Is(err, ErrNoBase) {
				t.Fatalf("base diff error = %v, want ErrNoBase", err)
			}
		})
	}
}
