package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/packages/go-sdk/procgroup"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
)

const reviewGitTimeout = 90 * time.Second
const reviewGitOutput = 8 << 10

type reviewGit struct {
	root   string
	origin string
}

func (g reviewGit) recheck(ctx context.Context, key string, credential credential, repository repository, revision wire.ReviewVersion, findings []reviewFindingSource, partial bool, policy projectCheckPolicy) wire.ReviewRecheck {
	result := wire.ReviewRecheck{
		PreviousRevision: revision.Recheck.PreviousRevision,
		Findings:         []wire.FindingRecheck{},
		FindingsPartial:  partial,
		Project:          copyProjectChecks(revision.Recheck.Project),
	}
	if err := g.verify(ctx, key, credential, repository, revision, findings, policy, &result); err != nil {
		result.Status = wire.ReviewRecheckFailed
		result.Diagnostic = err.Error()
		finishPendingProjectChecks(result.Project, wire.ProjectCheckNotRun, "")
		for _, source := range findings[len(result.Findings):] {
			result.Findings = append(result.Findings, wire.FindingRecheck{
				Finding: source.finding.ID, SourceRevision: source.revision.ID, Status: wire.FindingFileUnavailable,
			})
		}
	}
	result.CompletedAt = &instant.Timestamp{Time: time.Now().UTC()}
	return result
}

func (g reviewGit) verify(lifetime context.Context, key string, credential credential, repository repository, revision wire.ReviewVersion, findings []reviewFindingSource, policy projectCheckPolicy, result *wire.ReviewRecheck) (err error) {
	ctx, cancel := context.WithTimeout(lifetime, reviewGitTimeout)
	defer cancel()
	if !commitSHA.MatchString(revision.BaseSHA) || !commitSHA.MatchString(revision.HeadSHA) || len(revision.BaseSHA) != len(revision.HeadSHA) {
		return errors.New("github: the captured base and head must be exact commits of the same object format")
	}
	if err := os.MkdirAll(g.root, 0o700); err != nil {
		return fmt.Errorf("github: create review worktree root: %w", err)
	}
	root, err := os.OpenRoot(g.root)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	// The installation lease protects cleanup. A new path keeps an orphaned
	// Git process from reusing the next attempt's repository after a crash.
	if err := root.RemoveAll(key); err != nil {
		return err
	}
	if err := root.Mkdir(key, 0o700); err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.RemoveAll(key)) }()
	directory, err := os.MkdirTemp(filepath.Join(g.root, key), "attempt-")
	if err != nil {
		return fmt.Errorf("github: create review attempt directory: %w", err)
	}
	bare := filepath.Join(directory, "repository.git")
	checkout := filepath.Join(directory, "checkout")
	format := "sha1"
	if len(revision.HeadSHA) == 64 {
		format = "sha256"
	}
	if _, err := runReviewGit(ctx, directory, "", "init", "--quiet", "--bare", "--template=", "--object-format="+format, bare); err != nil {
		return fmt.Errorf("github: initialize review repository: %w", err)
	}
	remote := g.origin + "/" + repository.String() + ".git"
	authorization := ""
	if g.origin == "https://github.com" && credential.configured() {
		authorization = "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+credential.token))
	}
	if err := fetchReviewCommits(ctx, bare, remote, authorization, revision.BaseSHA, revision.HeadSHA); err != nil {
		return fmt.Errorf("github: fetch captured base and head: %w", err)
	}
	if _, err := runReviewGit(ctx, bare, "", "worktree", "add", "--detach", "--quiet", checkout, revision.HeadSHA); err != nil {
		return fmt.Errorf("github: create worktree at the captured head: %w", err)
	}
	head, err := runReviewGit(ctx, checkout, "", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return fmt.Errorf("github: verify worktree head: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(head), revision.HeadSHA) {
		return errors.New("github: worktree head differs from the captured commit")
	}
	result.CheckedHead = strings.TrimSpace(head)
	diagnostic, err := runReviewGit(ctx, checkout, "", "diff", "--no-ext-diff", "--no-textconv", "--check", revision.BaseSHA, revision.HeadSHA, "--")
	switch {
	case err == nil:
		result.Status = wire.ReviewRecheckClean
	case gitExit(err, 2):
		result.Status, result.Diagnostic = wire.ReviewRecheckIssues, diagnostic
	default:
		return fmt.Errorf("github: check captured diff: %w", err)
	}
	if len(findings) != 0 {
		recheckFindings(ctx, checkout, remote, authorization, revision, findings, result)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	policy.run(lifetime, filepath.Join(directory, "checks"), checkout, result.Project)
	return lifetime.Err()
}

func fetchReviewCommits(ctx context.Context, directory, remote, authorization string, commits ...string) error {
	for _, commit := range commits {
		if !commitSHA.MatchString(commit) {
			return errors.New("github: cannot fetch a nonexact commit")
		}
	}
	args := []string{"fetch", "--quiet", "--depth=1", "--no-tags", "--no-recurse-submodules", "--", remote}
	_, err := runReviewGit(ctx, directory, authorization, append(args, commits...)...)
	return err
}

func recheckFindings(ctx context.Context, directory, remote, authorization string, revision wire.ReviewVersion, findings []reviewFindingSource, result *wire.ReviewRecheck) {
	available := map[string]bool{revision.BaseSHA: true, revision.HeadSHA: true}
	for _, source := range findings {
		finding, previous := source.finding, source.revision
		before, after := previous.HeadSHA, revision.HeadSHA
		if finding.Side == wire.Left {
			before, after = previous.BaseSHA, revision.BaseSHA
		}
		if _, known := available[before]; !known {
			available[before] = fetchReviewCommits(ctx, directory, remote, authorization, before) == nil
		}
		recheck := wire.FindingRecheck{Finding: finding.ID, SourceRevision: previous.ID, Status: wire.FindingFileUnavailable}
		if available[before] {
			recheck.Status, recheck.Patch, recheck.PatchTruncated = compareFindingFile(ctx, directory, before, after, finding.Path)
		}
		result.Findings = append(result.Findings, recheck)
	}
}

func compareFindingFile(ctx context.Context, directory, before, after, path string) (wire.FindingRecheckStatus, string, bool) {
	kind, err := runReviewGit(ctx, directory, "", "cat-file", "-t", before+":"+path)
	if err != nil || strings.TrimSpace(kind) != "blob" {
		return wire.FindingFileUnavailable, "", false
	}
	diff, err := runReviewGit(ctx, directory, "", "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--exit-code", before, after, "--", path)
	switch {
	case err == nil:
		return wire.FindingFileUnchanged, "", false
	case gitExit(err, 1):
		patch, truncated := limitReviewEvidence(diff, 1024)
		return wire.FindingFileChanged, patch, truncated
	default:
		return wire.FindingFileUnavailable, "", false
	}
}

func gitExit(err error, status int) bool {
	exit, ok := errors.AsType[*exec.ExitError](err)
	return ok && exit.ExitCode() == status
}

func runReviewGit(ctx context.Context, directory, authorization string, args ...string) (string, error) {
	options := []string{"--literal-pathspecs", "-c", "core.hooksPath=" + os.DevNull, "-c", "http.followRedirects=false", "-c", "credential.helper=", "-c", "protocol.ext.allow=never", "-c", "core.quotePath=true"}
	command := exec.CommandContext(ctx, "git", append(options, args...)...)
	command.Dir = directory
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"), "LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_TERMINAL_PROMPT=0",
	}
	if authorization != "" {
		command.Env = append(command.Env, "GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.https://github.com/.extraHeader", "GIT_CONFIG_VALUE_0="+authorization)
	}
	procgroup.Prepare(command)
	command.Cancel = func() error { return procgroup.Stop(command) }
	command.WaitDelay = 2 * time.Second
	output := capture.NewWriter(reviewGitOutput)
	command.Stdout, command.Stderr = output, output
	err := command.Run()
	if err != nil && ctx.Err() != nil {
		return "", ctx.Err()
	}
	diagnostic := output.Diagnostic()
	if output.Truncated() {
		diagnostic += "\nGit output was truncated."
	}
	return diagnostic, err
}
