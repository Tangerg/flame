package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const reviewFixture = "FLAME_TEST_GITHUB_REVIEW_URL"
const reviewFixtureDatabase = "FLAME_TEST_GITHUB_REVIEW_DATABASE"
const reviewFixtureGitOrigin = "FLAME_TEST_GITHUB_REVIEW_GIT_ORIGIN"

func TestMain(tests *testing.M) {
	if endpoint := os.Getenv("FLAME_TEST_GITHUB_ISSUE_ENDPOINT"); endpoint != "" {
		backend.Main("github", func(ctx context.Context, data string) error {
			address, err := url.Parse(endpoint)
			if err != nil {
				return err
			}
			transport := roundTrip(func(request *http.Request) (*http.Response, error) {
				request = request.Clone(request.Context())
				request.URL.Scheme, request.URL.Host = address.Scheme, address.Host
				return http.DefaultTransport.RoundTrip(request)
			})
			return serveGitHub(ctx, data, transport, os.Getenv("FLAME_TEST_GITHUB_ISSUE_GIT_ORIGIN"))
		})
		os.Exit(0)
	}
	if endpoint := os.Getenv(reviewFixture); endpoint != "" {
		if err := runReviewFixture(endpoint, os.Getenv(reviewFixtureDatabase)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(tests.Run())
}

func runReviewFixture(endpoint, database string) error {
	address, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	ctx := context.Background()
	store, err := openStore(ctx, database)
	if err != nil {
		return err
	}
	defer store.db.Close()
	token := "local-review-fixture"
	credential, err := newCredential(&token)
	if err != nil {
		return err
	}
	connector, err := newConnector(store, roundTrip(func(request *http.Request) (*http.Response, error) {
		testRequest := request.Clone(request.Context())
		testRequest.URL.Scheme, testRequest.URL.Host = address.Scheme, address.Host
		return http.DefaultTransport.RoundTrip(testRequest)
	}), credential)
	if err != nil {
		return err
	}
	if err := connector.configureReview(); err != nil {
		return err
	}
	if origin := os.Getenv(reviewFixtureGitOrigin); origin != "" {
		connector.reviewGit.origin = origin
		interval := 25 * time.Millisecond
		if connector.webhook.port != 0 {
			interval = time.Hour
		}
		background, err := connector.reviewBackground(ctx, interval, nil, func(ctx context.Context) error {
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/review-notice", nil)
			if err != nil {
				return err
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				return err
			}
			return response.Body.Close()
		}, func() error { return nil })
		if err != nil {
			return err
		}
		defer background.Close()
		if err := background.Start(); err != nil {
			return err
		}
	}
	server, operations, err := newServer(connector)
	if err != nil {
		return err
	}
	defer operations.Close()
	return server.Run(ctx, &sdk.StdioTransport{})
}

func reviewProcess(t *testing.T, address, database, gitOrigin string, environment ...string) (*sdk.ClientSession, *exec.Cmd) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary)
	command.Env = []string{reviewFixture + "=" + address, reviewFixtureDatabase + "=" + database,
		reviewFixtureGitOrigin + "=" + gitOrigin, "PATH=" + os.Getenv("PATH")}
	command.Env = append(command.Env, environment...)
	client, err := pluginmcp.NewProcessClient()
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, command
}

type reviewFixtureSource struct {
	base        string
	heads       [3]string
	headIndex   atomic.Int32
	reviewCount atomic.Int32
	checkReads  atomic.Int32
	polled      chan int
}

func (s *reviewFixtureSource) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	current := s.headIndex.Load()
	head, before, after := s.heads[current], s.heads[0], s.heads[1]
	w.Header().Set("Content-Type", "application/json")
	switch request.URL.Path {
	case "/repos/owner/repo/pulls/42", "/repos/owner/repo/pulls/43":
		number := 42
		if strings.HasSuffix(request.URL.Path, "/43") {
			number, head = 43, s.heads[2]
		}
		fmt.Fprintf(w, `{"number":%d,"title":"Review %d","html_url":"https://github.com/owner/repo/pull/%d","state":"open","draft":false,"head":{"ref":"topic","sha":%q,"repo":{"full_name":"owner/repo"}},"base":{"ref":"main","sha":%q,"repo":{"full_name":"owner/repo"}},"updated_at":"2026-11-20T00:00:00Z","additions":1,"deletions":0,"changed_files":1}`, number, number, number, head, s.base)
		select {
		case s.polled <- number:
		default:
		}
	case "/repos/owner/repo/commits/" + before + "/check-runs":
		if current > 0 {
			fmt.Fprintf(w, `{"total_count":1,"check_runs":[%s]}`, reviewCheckJSON(10, before, "completed", "success"))
		} else {
			fmt.Fprint(w, `{"total_count":0,"check_runs":[]}`)
		}
	case "/repos/owner/repo/commits/" + after + "/check-runs":
		s.checkReads.Add(1)
		fmt.Fprintf(w, `{"total_count":2,"check_runs":[%s,%s]}`,
			reviewCheckJSON(10, before, "completed", "success"), reviewCheckJSON(11, after, "in_progress", ""))
	case "/repos/owner/repo/commits/" + s.heads[2] + "/check-runs":
		fmt.Fprint(w, `{"total_count":0,"check_runs":[]}`)
	case "/repos/owner/repo/issues/42/comments":
		if current > 0 {
			fmt.Fprint(w, `[{"id":30,"html_url":"https://github.com/owner/repo/pull/42#issuecomment-30","body":"Please recheck on the new commit.","user":{"login":"reviewer"},"created_at":"2026-11-20T00:00:00Z","updated_at":"2026-11-20T00:00:00Z"}]`)
		} else {
			fmt.Fprint(w, `[]`)
		}
	case "/repos/owner/repo/issues/43/comments":
		fmt.Fprint(w, `[]`)
	case "/repos/owner/repo/pulls/42/reviews":
		if request.Method != "POST" {
			http.Error(w, "expected review POST", http.StatusMethodNotAllowed)
			return
		}
		s.reviewCount.Add(1)
		var body struct {
			Commit string `json:"commit_id"`
		}
		if json.NewDecoder(request.Body).Decode(&body) != nil || (body.Commit != after && body.Commit != s.heads[2]) {
			http.Error(w, "expected exact reviewed SHA", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"id":40,"html_url":"https://github.com/owner/repo/pull/42#pullrequestreview-40","body":"Checked this revision.","state":"COMMENTED","commit_id":%q,"submitted_at":"2026-11-20T00:00:00Z"}`, body.Commit)
	default:
		http.NotFound(w, request)
	}
}

func reviewCheckJSON(id int, head, status, conclusion string) string {
	verdict := "null"
	if conclusion != "" {
		verdict = fmt.Sprintf("%q", conclusion)
	}
	return fmt.Sprintf(`{"id":%d,"head_sha":%q,"name":"CI","status":%q,"conclusion":%s,"output":{"annotations_count":0}}`, id, head, status, verdict)
}

func reviewRequest[P, R any](t *testing.T, session *sdk.ClientSession, contracts rpc.Contracts, method *pluginmcp.Method[P, R], key string, request P) R {
	t.Helper()
	result, err := reviewResult(t, session, contracts, method, key, request)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func reviewResult[P, R any](t *testing.T, session *sdk.ClientSession, contracts rpc.Contracts, method *pluginmcp.Method[P, R], key string, request P) (R, error) {
	t.Helper()
	ctx, err := pluginmcp.WithInvocation(t.Context(), framecall.Invocation{ID: "review-" + key, Origin: extension.Human, Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: "github-owner"}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := framevalue.EncodeJSON(request)
	if err != nil {
		t.Fatal(err)
	}
	var command pluginmcp.CommandID
	if method.Descriptor().Kind == "command" {
		command = pluginmcp.CommandID("review-" + key)
	}
	var result R
	response, err := githubBinding(t, session, contracts, method.Name()).Invoke(ctx, command, input)
	if err != nil {
		return result, err
	}
	err = response.Decode(&result)
	return result, err
}

func TestReviewWorkKeepsCommitEvidenceAndDiscussionAcrossRestart(t *testing.T) {
	source := &reviewFixtureSource{
		base:  strings.Repeat("1", 40),
		heads: [3]string{strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)},
	}
	server := httptest.NewServer(source)
	t.Cleanup(server.Close)
	database := filepath.Join(t.TempDir(), "github.db")
	session, _ := reviewProcess(t, server.URL, database, "")
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	first := wire.ReviewWorkInput{IssueInput: wire.IssueInput{Repository: "owner/repo", Number: 42}}
	second := wire.ReviewWorkInput{IssueInput: wire.IssueInput{Repository: "owner/repo", Number: 43}}
	observe := func(input wire.ReviewWorkInput, version int64, key string) wire.ReviewWork {
		return reviewRequest(t, session, contracts, contracts.UpdateReviewWork, key, wire.ReviewWorkChange{ReviewWorkInput: input, ExpectedVersion: version})
	}
	work := observe(first, 0, "inspect-a")
	initial := work.Latest
	secondWork := observe(second, 0, "inspect-other")
	if work.Latest == secondWork.Latest || secondWork.Number != second.Number {
		t.Fatal("two pull requests shared a review")
	}
	first.Revision = initial
	for index, disposition := range []wire.FindingDisposition{wire.FindingAccepted, wire.FindingDeferred} {
		key := fmt.Sprintf("finding-%d", index)
		work = reviewRequest(t, session, contracts, contracts.UpdateReviewWork, key, wire.ReviewWorkChange{
			ReviewWorkInput: first, ExpectedVersion: work.Version,
			Finding: &wire.ReviewFindingInput{Path: "main.go", Side: wire.Right, Line: int64(index + 1), Summary: key},
		})
		work = reviewRequest(t, session, contracts, contracts.UpdateReviewWork, key+"-decision", wire.ReviewWorkChange{
			ReviewWorkInput: first, ExpectedVersion: work.Version,
			Decision: &wire.FindingDecisionInput{Finding: work.Findings[index].ID, Disposition: disposition, Reason: "Inspected on revision A"},
		})
	}
	source.headIndex.Store(1)
	latest := first
	latest.Revision = ""
	work = observe(latest, work.Version, "inspect-b")
	if work.Latest == initial || len(work.NeedsRecheck) != 2 || len(work.Findings) != 0 ||
		len(work.Checks) != 1 || work.Checks[0].HeadSHA != work.Selected.HeadSHA ||
		work.Checks[0].Conclusion != nil || !work.ChecksPartial || len(work.Discussion) != 1 {
		t.Fatalf("new revision mixed evidence or decisions: %+v", work)
	}
	history := observe(first, work.Version, "late-ci")
	if history.Latest != work.Latest || history.Selected.ID != initial || len(history.Checks) != 1 ||
		history.Checks[0].HeadSHA != history.Selected.HeadSHA ||
		history.Findings[0].Decisions[0].Disposition != wire.FindingAccepted ||
		history.Findings[1].Decisions[0].Disposition != wire.FindingDeferred {
		t.Fatalf("late CI changed head or erased original decisions: %+v", history)
	}
	work = observe(latest, history.Version, "duplicate-comment")
	if len(work.Discussion) != 1 || work.Discussion[0].ReceivedAtRevision != work.Latest {
		t.Fatalf("unexpected discussion deliveries: %+v", work.Discussion)
	}
	publication := wire.SubmitReviewInput{
		IssueInput: latest.IssueInput, CommitID: wire.CommitSHA(work.Selected.HeadSHA),
		Event: wire.CommentReview, Body: new("Checked this revision."),
	}
	if review := reviewRequest(t, session, contracts, contracts.SubmitReview, "publish", publication); review.ID != 40 {
		t.Fatalf("wrong publication acknowledgement: %+v", review)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	session, _ = reviewProcess(t, server.URL, database, "")
	original, err := framevalue.EncodeJSON(publication)
	if err != nil {
		t.Fatal(err)
	}
	raw, found, err := githubBinding(t, session, contracts, contracts.SubmitReview.Name()).ReadReceipt(t.Context(), "review-publish", original)
	var receipt wire.Review
	if err != nil || !found || raw.Decode(&receipt) != nil || receipt.ID != 40 || source.reviewCount.Load() != 1 {
		t.Fatalf("original publication was not recovered: found=%v calls=%d receipt=%+v err=%v", found, source.reviewCount.Load(), receipt, err)
	}
	reopened := reviewRequest(t, session, contracts, contracts.ReviewWork, "read-original", first)
	other := reviewRequest(t, session, contracts, contracts.ReviewWork, "read-other", second)
	if reopened.Selected.ID != initial || len(reopened.Findings) != 2 ||
		other.Version != secondWork.Version || len(other.Findings) != 0 || len(other.Discussion) != 0 {
		t.Fatalf("restart lost history or crossed pull requests: review=%+v other=%+v", reopened, other)
	}
}
