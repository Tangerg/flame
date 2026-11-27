package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
	framecall "github.com/Tangerg/frame/call"
)

const issueDeliveryVariable = "GITHUB_ISSUE_DELIVERY"

type issueDeliveryPolicy struct {
	Repository string `json:"repository"`
	ProjectID  string `json:"projectId"`
	Branch     string `json:"branch"`
	Label      string `json:"label"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	repo       repository
	digest     string
}

func (c *connector) configureIssueDelivery() error {
	raw := os.Getenv(issueDeliveryVariable)
	if raw == "" {
		return nil
	}
	if !c.credential.configured() {
		return errors.New("github: issue delivery requires an installation credential")
	}
	var policy issueDeliveryPolicy
	if err := json.Unmarshal([]byte(raw), &policy, json.RejectUnknownMembers(true)); err != nil {
		return errors.New("github: invalid issue delivery policy")
	}
	repo, err := newRepository(policy.Repository)
	if err != nil {
		return err
	}
	policy.Repository, policy.repo = repo.String(), repo
	if _, err := identity.ParseProject(policy.ProjectID); err != nil {
		return errors.New("github: issue delivery requires a project ID")
	}
	if !textPresent(policy.Label) || len(policy.Label) > 64 || strings.ContainsAny(policy.Label, ",\n\r") ||
		!validIssueBranch(policy.Branch) {
		return errors.New("github: issue delivery requires one label and one branch")
	}
	selection, err := model.New(policy.Provider, policy.Model)
	if err != nil || selection.ValidateExact() != nil {
		return errors.New("github: issue delivery requires an exact provider and model")
	}
	checks := c.checkPolicies[policy.Repository]
	if checks.digest == "" {
		return errors.New("github: issue delivery requires project checks for its repository")
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	policy.digest = digestText(string(data) + "\n" + checks.digest)
	c.issuePolicy = &policy
	return nil
}

func validIssueBranch(branch string) bool {
	if branch == "" || len(branch) > 240 || branch == "@" || strings.Contains(branch, "..") || strings.Contains(branch, "@{") ||
		strings.ContainsAny(branch, " ~^:?*[\\") || strings.ContainsFunc(branch, unicode.IsControl) {
		return false
	}
	for name := range strings.SplitSeq(branch, "/") {
		if name == "" || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") || strings.HasSuffix(name, ".lock") {
			return false
		}
	}
	return true
}

func (c *connector) issueScope() string {
	p := c.issuePolicy
	return c.credential.cacheKey("issue delivery\n" + p.Repository + "\n" + p.ProjectID)
}

func (p *issueDeliveryPolicy) matches(issue wire.Issue) bool {
	return issue.Repository == p.Repository && issue.State == wire.Open && !issue.PullRequest &&
		slices.ContainsFunc(issue.Labels, func(label string) bool { return strings.EqualFold(label, p.Label) })
}

func (c *connector) enqueueIssue(ctx context.Context, repositoryID, issueID, number int64) (bool, error) {
	p := c.issuePolicy
	item := issueDeliveryRecord{
		IssueDelivery: wire.IssueDelivery{
			ID: issueKey(p.ProjectID, repositoryID, issueID), IssueInput: wire.IssueInput{Repository: p.Repository, Number: number},
			Status: issueQueued, Branch: p.Branch,
		},
		RepoID: repositoryID, IssueID: issueID, Rule: p.digest,
	}
	return c.store.enqueueIssue(ctx, c.issueScope(), item)
}

func (c *connector) receiveIssueDelivery(w http.ResponseWriter, r *http.Request, raw []byte) {
	p := c.issuePolicy
	if p == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var event struct {
		Action string    `json:"action"`
		Issue  issueWire `json:"issue"`
		Repo   struct {
			ID       int64  `json:"id"`
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &event); err != nil || event.Repo.ID <= 0 || event.Issue.ID <= 0 || event.Issue.Number <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	repo, err := newRepository(event.Repo.FullName)
	if err != nil || repo != p.repo || event.Issue.PullRequest != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if event.Action != "opened" && event.Action != "labeled" && event.Action != "reopened" {
		c.wakeReviews()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	label := slices.ContainsFunc(event.Issue.Labels, func(label struct {
		Name string `json:"name"`
	}) bool {
		return strings.EqualFold(label.Name, p.Label)
	})
	if !label || event.Issue.State != wire.Open {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	created, err := c.enqueueIssue(r.Context(), event.Repo.ID, event.Issue.ID, event.Issue.Number)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	c.wakeReviews()
	if !created {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (c *connector) reconcileIssueDeliveries(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	p := c.issuePolicy
	repoID, err := c.issueRepositoryID(ctx)
	if err != nil {
		return false, err
	}
	filters := url.Values{"state": {"open"}, "labels": {p.Label}, "sort": {"created"}, "direction": {"asc"}}
	changed := false
	for next := new(int64(1)); next != nil; {
		page, err := readPage(ctx, c, p.repo.apiPath()+"/issues", filters, *next, 100, func(input issueWire) (wire.Issue, error) {
			return input.admit(p.repo)
		})
		if err != nil {
			return changed, err
		}
		for _, issue := range page.rows {
			if !p.matches(issue) {
				continue
			}
			created, err := c.enqueueIssue(ctx, repoID, issue.ID, issue.Number)
			if err != nil {
				return changed, err
			}
			changed = changed || created
		}
		next = page.next
	}
	return changed, nil
}

func (c *connector) issueRepositoryID(ctx context.Context) (int64, error) {
	return query(ctx, c, c.issuePolicy.repo.apiPath(), nil, func(value response) (int64, error) {
		var repo struct {
			ID       int64  `json:"id"`
			FullName string `json:"full_name"`
		}
		if err := json.Unmarshal(value.body, &repo); err != nil {
			return 0, err
		}
		name, err := newRepository(repo.FullName)
		if err != nil || name != c.issuePolicy.repo || repo.ID <= 0 {
			return 0, refused(refusalNotFound, "issue repository is no longer visible")
		}
		return repo.ID, nil
	})
}

func (c *connector) currentIssue(ctx context.Context, work issueDeliveryRecord, repoID int64) (wire.Issue, error) {
	if work.RepoID != repoID || work.Rule != c.issuePolicy.digest {
		return wire.Issue{}, refused(refusalNotFound, "issue delivery is no longer visible under this policy")
	}
	issue, err := c.getIssue(ctx, work.IssueInput)
	if err != nil {
		return wire.Issue{}, err
	}
	if issue.ID != work.IssueID || !c.issuePolicy.matches(issue) {
		return wire.Issue{}, refused(refusalNotFound, "issue delivery source is no longer authorized")
	}
	return issue, nil
}

func issueAccessLost(err error) bool {
	r, ok := errors.AsType[*framecall.Refusal](err)
	return ok && (r.Code == refusalNotFound || r.Code == refusalUnauthorized || r.Code == refusalForbidden)
}

func (c *connector) issueBranchHead(ctx context.Context) (string, error) {
	p := c.issuePolicy
	return query(ctx, c, p.repo.apiPath()+"/branches/"+url.PathEscape(p.Branch), nil, func(value response) (string, error) {
		var branch struct {
			Name   string `json:"name"`
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		if err := json.Unmarshal(value.body, &branch); err != nil {
			return "", err
		}
		if branch.Name != p.Branch || !commitSHA.MatchString(branch.Commit.SHA) {
			return "", errors.New("github: branch response lacks the configured ref and exact head")
		}
		return strings.ToLower(branch.Commit.SHA), nil
	})
}

func issueSourceVersion(issue wire.Issue) (string, error) {
	data, err := json.Marshal([2]string{issue.Title, issue.Body})
	return digestText(string(data)), err
}
