package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/plugins/github/internal/wire"
)

func (c *connector) prepareIssueWorktree(ctx context.Context, work issueDeliveryRecord) (checkout string, err error) {
	ctx, cancel := context.WithTimeout(ctx, reviewGitTimeout)
	defer cancel()
	if len(work.ID) != 64 || work.ID != issueKey(c.issuePolicy.ProjectID, work.RepoID, work.IssueID) {
		return "", errors.New("github: invalid issue worktree identity")
	}
	path := filepath.Join(c.store.data, "issue-worktrees")
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	// A checkout without a stored binding has never been dispatched. The
	// installation lease keeps another preparation from using that directory.
	if err := root.RemoveAll(work.ID); err != nil {
		return "", err
	}
	if err := root.Mkdir(work.ID, 0700); err != nil {
		return "", err
	}
	return c.reviewGit.checkout(ctx, filepath.Join(path, work.ID), c.credential, c.issuePolicy.repo, work.Head, work.Head)
}

func checkIssueWorktree(ctx context.Context, work issueDeliveryRecord) error {
	ctx, cancel := context.WithTimeout(ctx, reviewGitTimeout)
	defer cancel()
	head, err := runReviewGit(ctx, work.Workspace, "", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(head) != work.Head {
		return refused(refusalConflict, "verification worktree head changed")
	}
	status, err := runReviewGit(ctx, work.Workspace, "", "status", "--porcelain", "--untracked-files=all", "--ignored=matching", "--ignore-submodules=none")
	if err != nil {
		return err
	}
	if status != "" {
		return refused(refusalConflict, "verification worktree has unverified changes")
	}
	return nil
}

func (c *connector) issueCI(ctx context.Context, work issueDeliveryRecord) (wire.IssueVerificationCI, error) {
	const pageSize, maxPages = 32, 8
	ci := wire.IssueVerificationCI{Status: "incomplete", Checks: []wire.CheckRun{}}
	seen := make(map[int64]bool)
	var total int64
	next := int64(1)
	for page := 0; page < maxPages; page++ {
		part, err := c.readCheckPage(ctx, wire.CommitInput{
			Repository: work.Repository, Ref: work.Head, Page: next, Limit: pageSize,
		})
		if err != nil {
			return ci, err
		}
		if page == 0 {
			total = part.total
		}
		if part.total != total {
			return ci, nil
		}
		for _, check := range part.CheckRuns {
			if seen[check.ID] || check.HeadSHA != work.Head {
				return ci, nil
			}
			seen[check.ID] = true
			// The verdict needs the original identity and conclusion. Reports
			// retain bounded diagnostic text without copying a remote log.
			check.Summary, _ = limitReviewEvidence(check.Summary, projectCheckLogBytes)
			check.Text = ""
			ci.Checks = append(ci.Checks, check)
		}
		if part.NextPage != nil {
			next = *part.NextPage
			continue
		}
		if int64(len(ci.Checks)) == total {
			ci.Status = issueCIStatus(ci.Checks)
		}
		break
	}
	slices.SortFunc(ci.Checks, func(a, b wire.CheckRun) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return ci, nil
}

func issueCIStatus(checks []wire.CheckRun) string {
	if len(checks) == 0 {
		return "missing"
	}
	waiting := false
	for _, check := range checks {
		if check.Status != "completed" || check.Conclusion == nil {
			waiting = true
			continue
		}
		if *check.Conclusion != "success" {
			return "failed"
		}
	}
	if waiting {
		return "pending"
	}
	return "passed"
}

func issueLocalPassed(project *wire.ProjectVerification) bool {
	if project == nil || len(project.Checks) == 0 {
		return false
	}
	for _, check := range project.Checks {
		if check.Status != wire.ProjectCheckPassed || check.ExitCode == nil || *check.ExitCode != 0 {
			return false
		}
	}
	return true
}

func issueDraft(work issueDeliveryRecord) (string, string, error) {
	body := fmt.Sprintf("Flame verified `%s` on `%s`.\n\n%d local project checks passed in %s. %d GitHub check runs passed for this SHA.\n\nFlame Run `%s`.",
		work.Head, work.Branch, len(work.Project.Checks), work.Project.Environment, len(work.CI.Checks), work.RunID)
	evidence := struct {
		ID, Rule, Source, Head, Body string
		Project                      *wire.ProjectVerification
		CI                           *wire.IssueVerificationCI
	}{work.ID, work.Rule, work.Source, work.Head, body, work.Project, work.CI}
	raw, err := json.Marshal(evidence)
	return body, digestText(string(raw)), err
}
