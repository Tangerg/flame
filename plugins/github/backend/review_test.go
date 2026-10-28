package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"
	"github.com/Tangerg/flame/qimu/portable"
)

const pullJSON = `{"number":42,"title":"Review this change","body":null,"state":"open","html_url":"https://github.com/owner/repo/pull/42","user":{"login":"author"},"draft":true,"head":{"label":"contributor:topic","ref":"topic","sha":"head-sha","repo":null},"base":{"label":"owner:main","ref":"main","sha":"base-sha","repo":{"full_name":"owner/repo"}},"updated_at":"2026-10-20T00:00:00Z","merged_at":null,"mergeable":null,"mergeable_state":"unknown","additions":2,"deletions":1,"changed_files":2}`
const changedFileJSON = `{"filename":"new.go","previous_filename":"old.go","sha":"file-sha","status":"renamed","additions":2,"deletions":1,"changes":3,"patch":"@@ -1 +1,2 @@\n-old\n+new\n+line"}`
const commentJSON = `{"id":11,"html_url":"https://github.com/owner/repo/issues/42#issuecomment-11","user":null,"body":"Please check this","created_at":"2026-10-20T00:00:00Z","updated_at":"2026-10-20T01:00:00Z"}`
const reviewJSON = `{"id":22,"html_url":"https://github.com/owner/repo/pull/42#pullrequestreview-22","user":{"login":"reviewer"},"body":"","state":"PENDING","commit_id":null,"submitted_at":null}`
const reviewCommentJSON = `{"id":33,"html_url":"https://github.com/owner/repo/pull/42#discussion_r33","user":{"login":"reviewer"},"body":"Outdated line","created_at":"2026-10-20T00:00:00Z","updated_at":"2026-10-20T01:00:00Z","pull_request_review_id":22,"path":"new.go","diff_hunk":"@@ -1 +1,2 @@","commit_id":"head-sha","original_commit_id":"old-head-sha","line":null,"original_line":7,"side":"RIGHT","start_line":null,"start_side":null,"in_reply_to_id":32}`
const checkJSON = `{"id":44,"name":"build","head_sha":"head-sha","status":"in_progress","conclusion":null,"html_url":"https://github.com/owner/repo/actions/runs/44","started_at":"2026-10-20T00:00:00Z","completed_at":null,"output":{"title":null,"summary":null,"text":null,"annotations_count":1}}`
const annotationJSON = `{"path":"new.go","start_line":4,"end_line":4,"start_column":2,"end_column":5,"annotation_level":"failure","message":"Unexpected value","title":"Build failed","raw_details":"Original compiler output"}`
const statusJSON = `{"id":55,"state":"pending","context":"external/build","description":null,"target_url":null,"created_at":"2026-10-20T00:00:00Z","updated_at":"2026-10-20T01:00:00Z"}`

func reviewConnector(t *testing.T, transport http.RoundTripper) *connector {
	t.Helper()
	cache, err := openStore(t.Context(), filepath.Join(t.TempDir(), "github.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.db.Close() })
	c, err := newConnector(cache, transport, credential{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPullReadsPreserveUnknownAndUnavailableUpstreamFacts(t *testing.T) {
	c := reviewConnector(t, roundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/repos/owner/repo/pulls":
			q := r.URL.Query()
			if q.Get("base") != "release/v2" || q.Get("head") != "contributor:topic" || q.Get("state") != "closed" || q.Get("page") != "2" || q.Get("per_page") != "10" {
				t.Errorf("PR list lost its filters: %s", r.URL)
			}
			return upstream(200, "["+pullJSON+"]", http.Header{"Link": {`<https://api.github.com/repositories/1/pulls?page=3>; rel="next"`}}), nil
		case "/repos/owner/repo/pulls/42":
			return upstream(200, pullJSON, nil), nil
		case "/repos/owner/repo/pulls/42/files":
			return upstream(200, "["+changedFileJSON+`,{"filename":"image.png","sha":"binary-sha","status":"added","additions":0,"deletions":0,"changes":0}]`, nil), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, nil
		}
	}))
	list, err := c.listPulls(t.Context(), contractwire.PullListInput{ListInput: contractwire.ListInput{Repository: "Owner/Repo", State: "closed", Page: 2, Limit: 10}, Base: new("release/v2"), Head: new("contributor:topic")})
	if err != nil || len(list.PullRequests) != 1 || list.NextPage == nil || *list.NextPage != 3 {
		t.Fatalf("pull list = %+v, %v", list, err)
	}
	pull, err := c.getPull(t.Context(), contractwire.IssueInput{Repository: "owner/repo", Number: 42})
	if err != nil || pull.Head.SHA != "head-sha" || pull.Base.SHA != "base-sha" || pull.Head.Repository != nil || pull.Mergeable != nil || !pull.Draft || pull.ChangedFiles == nil || *pull.ChangedFiles != 2 {
		t.Fatalf("PR revision or unavailable facts changed: %+v, %v", pull, err)
	}
	files, err := c.listPullFiles(t.Context(), contractwire.ThreadInput{IssueInput: contractwire.IssueInput{Repository: "owner/repo", Number: 42}, Page: 1, Limit: 10})
	if err != nil || len(files.Files) != 2 {
		t.Fatalf("files = %+v, %v", files, err)
	}
	if files.Files[0].PreviousPath == nil || *files.Files[0].PreviousPath != "old.go" || files.Files[0].Patch == nil || !strings.Contains(*files.Files[0].Patch, "+new") || files.Files[1].Patch != nil || files.LimitReached {
		t.Fatalf("rename, patch or completeness evidence changed: %+v", files)
	}
}

func TestPullFileCeilingCannotLookLikeAnUnboundedContinuation(t *testing.T) {
	calls := 0
	c := reviewConnector(t, roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Get("page") != "30" {
			t.Fatalf("requested a page beyond GitHub's file ceiling: %s", r.URL)
		}
		rows := make([]jsontext.Value, 100)
		for i := range rows {
			rows[i] = jsontext.Value(changedFileJSON)
		}
		body, err := json.Marshal(rows)
		return upstream(200, string(body), http.Header{"Link": {`<https://api.github.com/repos/owner/repo/pulls/42/files?page=31>; rel="next"`}}), err
	}))
	input := contractwire.ThreadInput{IssueInput: contractwire.IssueInput{Repository: "owner/repo", Number: 42}, Page: 30, Limit: 100}
	page, err := c.listPullFiles(t.Context(), input)
	if err != nil || !page.LimitReached || page.NextPage != nil || len(page.Files) != 100 {
		t.Fatalf("ceiling page = %+v, %v", page, err)
	}
	input.Page++
	_, err = c.listPullFiles(t.Context(), input)
	if refusal, ok := errors.AsType[*portable.Refusal](err); !ok || refusal.Code != "invalid_query" || calls != 1 {
		t.Fatalf("unsupported continuation = %v, calls=%d", err, calls)
	}
}

func TestReviewQueriesRejectMalformedPayloadsBeforeConditionalCaching(t *testing.T) {
	thread := contractwire.ThreadInput{IssueInput: contractwire.IssueInput{Repository: "owner/repo", Number: 42}, Page: 1, Limit: 1}
	commit := contractwire.CommitInput{Repository: "owner/repo", Ref: "head-sha", Page: 1, Limit: 1}
	for _, test := range []struct {
		name string
		body string
		call func(*connector) error
	}{
		{"null pull list", "null", func(c *connector) error {
			_, err := c.listPulls(t.Context(), contractwire.PullListInput{ListInput: contractwire.ListInput{Repository: "owner/repo", State: "all", Page: 1, Limit: 1}})
			return err
		}},
		{"wrong PR identity", strings.Replace(pullJSON, `"number":42`, `"number":43`, 1), func(c *connector) error { _, err := c.getPull(t.Context(), thread.IssueInput); return err }},
		{"missing PR statistics", strings.Replace(pullJSON, `,"changed_files":2`, "", 1), func(c *connector) error { _, err := c.getPull(t.Context(), thread.IssueInput); return err }},
		{"foreign base repository", strings.Replace(pullJSON, `"full_name":"owner/repo"`, `"full_name":"other/repo"`, 1), func(c *connector) error { _, err := c.getPull(t.Context(), thread.IssueInput); return err }},
		{"missing file change count", `[{"filename":"x","sha":"sha","status":"added"}]`, func(c *connector) error { _, err := c.listPullFiles(t.Context(), thread); return err }},
		{"oversized comment page", "[" + commentJSON + "," + commentJSON + "]", func(c *connector) error { _, err := c.listComments(t.Context(), thread); return err }},
		{"incomplete review", `[{"id":22,"state":"APPROVED"}]`, func(c *connector) error { _, err := c.listReviews(t.Context(), thread); return err }},
		{"missing inline context", "[" + commentJSON + "]", func(c *connector) error { _, err := c.listReviewComments(t.Context(), thread); return err }},
		{"null checks", `{"total_count":0,"check_runs":null}`, func(c *connector) error { _, err := c.listChecks(t.Context(), commit); return err }},
		{"inconsistent check count", `{"total_count":0,"check_runs":[` + checkJSON + "]}", func(c *connector) error { _, err := c.listChecks(t.Context(), commit); return err }},
		{"reversed annotation range", "[" + strings.Replace(annotationJSON, `"end_line":4`, `"end_line":3`, 1) + "]", func(c *connector) error {
			_, err := c.listAnnotations(t.Context(), contractwire.CheckInput{Repository: "owner/repo", CheckRunID: 44, Page: 1, Limit: 1})
			return err
		}},
		{"null status page", "null", func(c *connector) error { _, err := c.listStatuses(t.Context(), commit); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			c := reviewConnector(t, roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Header.Get("If-None-Match") != "" {
					t.Error("malformed upstream data became a cached representation")
				}
				return upstream(200, test.body, http.Header{"Etag": {`"invalid"`}}), nil
			}))
			for range 2 {
				if err := test.call(c); err == nil {
					t.Fatal("malformed upstream data became a successful query")
				} else if _, known := errors.AsType[*portable.Refusal](err); known {
					t.Fatalf("upstream protocol error became a caller refusal: %v", err)
				}
			}
			if calls != 2 {
				t.Fatalf("query did not revalidate upstream authority: %d", calls)
			}
		})
	}
}

func TestReviewAndCheckReadsRetainDiagnosticContext(t *testing.T) {
	fixtures := map[string]string{
		"/repos/owner/repo/issues/42/comments":          "[" + commentJSON + "]",
		"/repos/owner/repo/pulls/42/reviews":            "[" + reviewJSON + "]",
		"/repos/owner/repo/pulls/42/comments":           "[" + reviewCommentJSON + "]",
		"/repos/owner/repo/commits/head-sha/check-runs": `{"total_count":1,"check_runs":[` + checkJSON + "]}",
		"/repos/owner/repo/check-runs/44/annotations":   "[" + annotationJSON + "]",
		"/repos/owner/repo/commits/head-sha/statuses":   "[" + statusJSON + "]",
	}
	c := reviewConnector(t, roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("page") != "1" || r.URL.Query().Get("per_page") != "10" {
			t.Errorf("pagination changed: %s", r.URL)
		}
		if strings.HasSuffix(r.URL.Path, "/check-runs") && r.URL.Query().Get("filter") != "latest" {
			t.Error("checks lost their explicit latest filter")
		}
		body, ok := fixtures[r.URL.Path]
		if !ok {
			t.Fatalf("unexpected diagnostic request: %s", r.URL)
		}
		return upstream(200, body, nil), nil
	}))
	thread := contractwire.ThreadInput{IssueInput: contractwire.IssueInput{Repository: "owner/repo", Number: 42}, Page: 1, Limit: 10}
	comments, err := c.listComments(t.Context(), thread)
	if err != nil || len(comments.Comments) != 1 || comments.Comments[0].Author != "" || comments.Comments[0].Body != "Please check this" {
		t.Fatalf("discussion = %+v, %v", comments, err)
	}
	reviews, err := c.listReviews(t.Context(), thread)
	if err != nil || len(reviews.Reviews) != 1 || reviews.Reviews[0].State != "PENDING" || reviews.Reviews[0].SubmittedAt != nil || reviews.Reviews[0].CommitID != nil {
		t.Fatalf("pending review = %+v, %v", reviews, err)
	}
	inline, err := c.listReviewComments(t.Context(), thread)
	if err != nil || len(inline.Comments) != 1 || inline.Comments[0].Line != nil || inline.Comments[0].OriginalLine == nil || *inline.Comments[0].OriginalLine != 7 || inline.Comments[0].InReplyTo == nil || *inline.Comments[0].InReplyTo != 32 || inline.Comments[0].OriginalCommitID != "old-head-sha" {
		t.Fatalf("outdated inline context = %+v, %v", inline, err)
	}
	commit := contractwire.CommitInput{Repository: "owner/repo", Ref: "head-sha", Page: 1, Limit: 10}
	checks, err := c.listChecks(t.Context(), commit)
	if err != nil || len(checks.CheckRuns) != 1 || checks.CheckRuns[0].HeadSHA != "head-sha" || checks.CheckRuns[0].Conclusion != nil || checks.CheckRuns[0].Status != "in_progress" || checks.CheckRuns[0].AnnotationsCount != 1 {
		t.Fatalf("unfinished check = %+v, %v", checks, err)
	}
	annotations, err := c.listAnnotations(t.Context(), contractwire.CheckInput{Repository: "owner/repo", CheckRunID: checks.CheckRuns[0].ID, Page: 1, Limit: 10})
	if err != nil || len(annotations.Annotations) != 1 || annotations.Annotations[0].StartLine != 4 || annotations.Annotations[0].Level != "failure" || annotations.Annotations[0].Message != "Unexpected value" || annotations.Annotations[0].RawDetails == nil {
		t.Fatalf("line diagnostic = %+v, %v", annotations, err)
	}
	statuses, err := c.listStatuses(t.Context(), commit)
	if err != nil || len(statuses.Statuses) != 1 || statuses.Statuses[0].Context != "external/build" || statuses.Statuses[0].State != "pending" || statuses.Statuses[0].URL != nil {
		t.Fatalf("external status = %+v, %v", statuses, err)
	}
}
