package main

import (
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	storage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
	"github.com/Tangerg/frame/sqlite"
)

const reviewWorkSchema = `CREATE TABLE IF NOT EXISTS review_work (scope TEXT PRIMARY KEY, snapshot TEXT NOT NULL CHECK(json_valid(snapshot)))`

const (
	maxReviewVersions   = 32
	maxRevisionFindings = 32
	maxDiscussionEvents = 64
	maxReviewWorkBytes  = 2 << 20
)

type reviewRevision struct {
	wire.ReviewVersion
	Checks        []wire.ReviewCheck   `json:"checks"`
	ChecksPartial bool                 `json:"checksPartial"`
	NeedsEvidence bool                 `json:"needsEvidence,omitempty"`
	Findings      []wire.ReviewFinding `json:"findings"`
}

type reviewSnapshot struct {
	Version    int64                   `json:"version"`
	Latest     string                  `json:"latest"`
	Revisions  []reviewRevision        `json:"revisions"`
	Discussion []wire.ReviewDiscussion `json:"discussion"`
	Follow     *reviewFollow           `json:"follow,omitzero"`
}

type reviewFollow struct {
	wire.IssueInput
	Enabled bool   `json:"enabled"`
	Error   string `json:"error,omitempty"`
}

func (s reviewSnapshot) revision(id string) int {
	if id == "" {
		id = s.Latest
	}
	return slices.IndexFunc(s.Revisions, func(value reviewRevision) bool { return value.ID == id })
}

func (s reviewSnapshot) view(pull wire.PullRequest, selected int) wire.ReviewWork {
	view := wire.ReviewWork{
		IssueInput: wire.IssueInput{Repository: pull.Repository, Number: pull.Number},
		URL:        pull.URL, Version: s.Version, Latest: s.Latest,
		CurrentHead: pull.Head.SHA,
		Versions:    make([]wire.ReviewVersion, 0, len(s.Revisions)),
		Checks:      []wire.ReviewCheck{}, Findings: []wire.ReviewFinding{},
		NeedsRecheck: []wire.ReviewFinding{}, Discussion: []wire.ReviewDiscussion{},
	}
	if s.Follow != nil {
		view.Following, view.FollowError = s.Follow.Enabled, s.Follow.Error
	}
	for _, item := range s.Revisions {
		view.Versions = append(view.Versions, item.ReviewVersion)
	}
	if selected >= 0 {
		revision := s.Revisions[selected]
		view.Selected = &revision.ReviewVersion
		view.Checks, view.ChecksPartial, view.Findings = revision.Checks, revision.ChecksPartial, revision.Findings
		if revision.Recheck != nil {
			if revision.Recheck.Status == wire.ReviewRecheckPending || len(revision.Recheck.Findings) == 0 {
				sources, _ := s.recheckSources(revision.Recheck.PreviousRevision)
				for _, source := range sources {
					view.NeedsRecheck = append(view.NeedsRecheck, source.finding)
				}
			} else {
				for _, check := range revision.Recheck.Findings {
					source, ok := s.findingSource(cmp.Or(check.SourceRevision, revision.Recheck.PreviousRevision), check.Finding)
					if ok {
						view.NeedsRecheck = append(view.NeedsRecheck, source.finding)
					}
				}
			}
		} else if selected > 0 {
			view.NeedsRecheck = s.Revisions[selected-1].Findings
		}
	}
	view.Discussion = append(view.Discussion, s.Discussion...)
	return view
}

func encodeReviewSnapshot(snapshot reviewSnapshot) ([]byte, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxReviewWorkBytes {
		return nil, refused(refusalNotAllowed, "review work storage capacity reached")
	}
	return raw, nil
}

func (s *store) saveReviewSnapshot(ctx context.Context, scope string, raw []byte) error {
	_, err := s.db.Executor(ctx).ExecContext(ctx,
		`INSERT INTO review_work(scope,snapshot) VALUES(?,?) ON CONFLICT(scope) DO UPDATE SET snapshot=excluded.snapshot`, scope, string(raw))
	return err
}

func (s *store) reviewSnapshot(ctx context.Context, scope string) (reviewSnapshot, error) {
	var snapshot reviewSnapshot
	err := s.db.Read(ctx, func(reader sqlite.Reader) error {
		var raw string
		if err := reader.QueryRowContext(ctx, `SELECT snapshot FROM review_work WHERE scope=?`, scope).Scan(&raw); err != nil {
			return err
		}
		return json.Unmarshal([]byte(raw), &snapshot, json.RejectUnknownMembers(true))
	})
	if errors.Is(err, sql.ErrNoRows) {
		return reviewSnapshot{}, nil
	}
	return snapshot, err
}

func (c *connector) reviewScope(repository string, number int64) string {
	return c.credential.cacheKey(fmt.Sprintf("review work\n%s\n%d", repository, number))
}

func (c *connector) readReviewWork(ctx context.Context, input wire.ReviewWorkInput) (wire.ReviewWork, error) {
	pull, err := c.getPull(ctx, input.IssueInput)
	if err != nil {
		return wire.ReviewWork{}, err
	}
	snapshot, err := c.store.reviewSnapshot(ctx, c.reviewScope(pull.Repository, pull.Number))
	if err != nil {
		return wire.ReviewWork{}, err
	}
	selected := snapshot.revision(input.Revision)
	if selected < 0 && input.Revision != "" {
		return wire.ReviewWork{}, refused(refusalNotFound, "review revision was not observed")
	}
	return snapshot.view(pull, selected), nil
}

type reviewObservation struct {
	Revision wire.ReviewVersion
	Checks   []wire.ReviewCheck
	Partial  bool
	Comments []wire.Comment
}

func (c *connector) inspectReview(ctx context.Context, input wire.ReviewWorkInput, pull wire.PullRequest) (reviewObservation, error) {
	if !commitSHA.MatchString(pull.Base.SHA) || !commitSHA.MatchString(pull.Head.SHA) {
		return reviewObservation{}, errors.New("github: pull request has no exact base and head commits")
	}
	version := wire.ReviewVersion{
		ID:      pull.Base.SHA + ":" + pull.Head.SHA,
		BaseSHA: pull.Base.SHA, HeadSHA: pull.Head.SHA, Title: pull.Title,
	}
	if pull.Head.Repository != nil {
		version.HeadRepository = *pull.Head.Repository
	}
	if input.Revision != "" && input.Revision != version.ID {
		snapshot, err := c.store.reviewSnapshot(ctx, c.reviewScope(pull.Repository, pull.Number))
		if err != nil {
			return reviewObservation{}, err
		}
		selected := snapshot.revision(input.Revision)
		if selected < 0 {
			return reviewObservation{}, refused(refusalNotFound, "review revision was not observed")
		}
		version = snapshot.Revisions[selected].ReviewVersion
	}
	return c.loadReviewEvidence(ctx, input.IssueInput, version, input.Revision == "")
}

func (c *connector) loadReviewEvidence(ctx context.Context, target wire.IssueInput, version wire.ReviewVersion, discussion bool) (reviewObservation, error) {
	checks, err := c.listChecks(ctx, wire.CommitInput{Repository: target.Repository, Ref: version.HeadSHA, Page: 1, Limit: 32})
	if err != nil {
		return reviewObservation{}, err
	}
	observed := reviewObservation{
		Revision: version, Checks: []wire.ReviewCheck{},
		Partial: checks.NextPage != nil,
	}
	if discussion {
		comments, err := c.listComments(ctx, wire.ThreadInput{IssueInput: target, Page: 1, Limit: 32})
		if err != nil {
			return reviewObservation{}, err
		}
		observed.Comments = comments.Comments
	}
	for _, check := range checks.CheckRuns {
		if check.HeadSHA != version.HeadSHA {
			observed.Partial = true
			continue
		}
		observed.Checks = append(observed.Checks, wire.ReviewCheck{
			ID: check.ID, Name: check.Name, HeadSHA: check.HeadSHA,
			Status: check.Status, Conclusion: check.Conclusion, URL: check.URL,
		})
	}
	return observed, nil
}

func (s *reviewSnapshot) retainRevision(version wire.ReviewVersion) (int, error) {
	position := s.revision(version.ID)
	if position == -1 {
		if len(s.Revisions) >= maxReviewVersions {
			return -1, refused(refusalNotAllowed, "review revision capacity reached")
		}
		position = len(s.Revisions)
		s.Revisions = append(s.Revisions, reviewRevision{
			ReviewVersion: version, Checks: []wire.ReviewCheck{}, Findings: []wire.ReviewFinding{},
		})
	}
	return position, nil
}

func (s *reviewSnapshot) observe(input wire.ReviewWorkInput, observation reviewObservation, policy projectCheckPolicy) (int, error) {
	position, err := s.retainRevision(observation.Revision)
	if err != nil {
		return -1, err
	}
	revision := &s.Revisions[position]
	revision.recordEvidence(observation)
	if input.Revision == "" {
		if s.Follow != nil && s.Follow.Enabled && s.Latest != "" && s.Latest != revision.ID && revision.Recheck == nil {
			revision.scheduleRecheck(s.Latest, policy)
		}
		if err := s.recordCurrent(revision.ID, observation.Comments); err != nil {
			return -1, err
		}
	}
	return position, nil
}

func (r *reviewRevision) recordEvidence(observation reviewObservation) {
	r.Checks, r.ChecksPartial, r.NeedsEvidence = observation.Checks, observation.Partial, false
}

func (r *reviewRevision) scheduleRecheck(previous string, policy projectCheckPolicy) {
	r.Recheck = &wire.ReviewRecheck{
		Status: wire.ReviewRecheckPending, PreviousRevision: previous, Findings: []wire.FindingRecheck{}, Project: policy.capture(),
	}
}

func (s *reviewSnapshot) recordCurrent(id string, comments []wire.Comment) error {
	s.Latest = id
	for _, comment := range comments {
		if slices.ContainsFunc(s.Discussion, func(seen wire.ReviewDiscussion) bool {
			return seen.ID == comment.ID && seen.UpdatedAt.Equal(comment.UpdatedAt.Time) && seen.Body == comment.Body
		}) {
			continue
		}
		if len(s.Discussion) >= maxDiscussionEvents {
			return refused(refusalNotAllowed, "review discussion capacity reached")
		}
		s.Discussion = append(s.Discussion, wire.ReviewDiscussion{Comment: comment, ReceivedAtRevision: id})
	}
	return nil
}

func (s *reviewSnapshot) edit(input wire.ReviewWorkChange, command string) (int, error) {
	selected := s.revision(input.Revision)
	if input.Revision == "" || selected < 0 {
		return -1, refused(refusalNotFound, "an observed review revision is required")
	}
	revision := &s.Revisions[selected]
	if finding := input.Finding; finding != nil {
		if _, err := newFilePath(finding.Path); err != nil {
			return -1, err
		}
		if !finding.Side.Valid() || finding.Line <= 0 || strings.TrimSpace(finding.Summary) == "" ||
			finding.Side == wire.Right && revision.HeadRepository == "" {
			return -1, invalidInput("finding must locate a line on an available review side")
		}
		if len(revision.Findings) >= maxRevisionFindings {
			return -1, refused(refusalNotAllowed, "review finding capacity reached")
		}
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(command)))
		revision.Findings = append(revision.Findings, wire.ReviewFinding{
			ReviewFindingInput: *finding, ID: id, Decisions: []wire.FindingDecision{},
		})
		return selected, nil
	}
	decision := input.Decision
	index := slices.IndexFunc(revision.Findings, func(finding wire.ReviewFinding) bool { return finding.ID == decision.Finding })
	var decisions *[]wire.FindingDecision
	if index >= 0 {
		decisions = &revision.Findings[index].Decisions
	}
	if decisions == nil && revision.Recheck != nil {
		for i := range revision.Recheck.Findings {
			if revision.Recheck.Findings[i].Finding == decision.Finding {
				decisions = &revision.Recheck.Findings[i].Decisions
				break
			}
		}
	}
	if decisions == nil {
		return -1, refused(refusalNotFound, "finding is absent from the inspected revision")
	}
	if strings.TrimSpace(decision.Reason) == "" ||
		decision.Disposition != wire.FindingAccepted && decision.Disposition != wire.FindingDeferred &&
			(decision.Disposition != wire.FindingResolved || index >= 0) {
		return -1, invalidInput("a disposition and its reason are required")
	}
	*decisions = append(*decisions, wire.FindingDecision{
		Disposition: decision.Disposition, Reason: decision.Reason, At: instant.Timestamp{Time: time.Now().UTC()},
	})
	return selected, nil
}

func (c *connector) changeReviewWork(ctx context.Context, command pluginmcp.Command[wire.ReviewWorkChange], method string) (wire.ReviewWork, error) {
	input := command.Input
	if input.Finding != nil && input.Decision != nil ||
		input.Follow != nil && (input.Finding != nil || input.Decision != nil) {
		return wire.ReviewWork{}, invalidInput("choose one review change")
	}
	if input.Decision != nil || input.Follow != nil {
		invocation, err := pluginmcp.InvocationFromContext(ctx)
		if err != nil {
			return wire.ReviewWork{}, err
		}
		if invocation.Origin != extension.Human {
			return wire.ReviewWork{}, refused(refusalNotAllowed, "a person must decide findings and follow settings")
		}
	}
	pull, err := c.getPull(ctx, input.IssueInput)
	if err != nil {
		return wire.ReviewWork{}, err
	}
	var observed reviewObservation
	if input.Follow == nil && input.Finding == nil && input.Decision == nil {
		observed, err = c.inspectReview(ctx, input.ReviewWorkInput, pull)
		if err != nil {
			return wire.ReviewWork{}, err
		}
	}
	admitted, err := command.RequestInput()
	if err != nil {
		return wire.ReviewWork{}, err
	}
	scope := c.reviewScope(pull.Repository, pull.Number)
	policy := c.checkPolicies[pull.Repository]
	result, err := storage.Command(ctx, c.store.db, method, string(command.ID), admitted, classify, func(ctx context.Context, input wire.ReviewWorkChange) (storage.CommandPlan[wire.ReviewWork], error) {
		var empty storage.CommandPlan[wire.ReviewWork]
		snapshot, err := c.store.reviewSnapshot(ctx, scope)
		if err != nil {
			return empty, err
		}
		if input.ExpectedVersion != snapshot.Version {
			return empty, refused(refusalConflict, "review work changed since it was inspected")
		}
		var selected int
		switch {
		case input.Follow != nil:
			selected = snapshot.revision(input.Revision)
			if selected < 0 {
				return empty, refused(refusalNotFound, "inspect a review revision before changing its follow setting")
			}
			snapshot.Follow = &reviewFollow{IssueInput: wire.IssueInput{Repository: pull.Repository, Number: pull.Number}, Enabled: *input.Follow}
		case input.Finding == nil && input.Decision == nil:
			selected, err = snapshot.observe(input.ReviewWorkInput, observed, policy)
		default:
			selected, err = snapshot.edit(input, string(command.ID))
		}
		if err != nil {
			return empty, err
		}
		snapshot.Version++
		encoded, err := encodeReviewSnapshot(snapshot)
		if err != nil {
			return empty, err
		}
		return storage.NewCommandPlan(snapshot.view(pull, selected), func(ctx context.Context) error {
			return c.store.saveReviewSnapshot(ctx, scope, encoded)
		})
	})
	if err == nil && (input.Follow != nil || input.Revision == "" && input.Finding == nil && input.Decision == nil) {
		c.wakeReviews()
	}
	return result, err
}
