package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
	"github.com/Tangerg/flame/plugins/github/internal/wire"
	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type followFixture struct {
	source    *reviewFixtureSource
	server    *httptest.Server
	contracts rpc.Contracts
	database  string
	gitOrigin string
	notices   chan struct{}
	hold      atomic.Bool
}

func newFollowFixture(t *testing.T) *followFixture {
	t.Helper()
	base, heads, origin := reviewGitFixture(t)
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	f := &followFixture{
		source:    &reviewFixtureSource{base: base, heads: heads, polled: make(chan int, 16)},
		contracts: contracts,
		database:  filepath.Join(t.TempDir(), "github.db"),
		gitOrigin: origin,
		notices:   make(chan struct{}, 8),
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/review-notice" {
			f.source.ServeHTTP(w, request)
			return
		}
		f.notices <- struct{}{}
		if f.hold.Load() {
			<-request.Context().Done()
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func reviewGitFixture(t *testing.T) (string, [3]string, string) {
	t.Helper()
	repo := t.TempDir()
	remotes := t.TempDir()
	if err := os.Mkdir(filepath.Join(remotes, "owner"), 0o700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{
			"-c", "user.name=Review Fixture", "-c", "user.email=review@example.test",
			"-c", "core.hooksPath=" + os.DevNull,
		}, args...)...)
		cmd.Dir = repo
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "--quiet", "--template=", "--object-format=sha1", "--initial-branch=topic")
	if err := os.WriteFile(filepath.Join(repo, "steady.txt"), []byte("Unchanged reference.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	commit := func(text string) string {
		if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package review\n\n// "+text+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		git("add", "--", "main.go", "steady.txt")
		git("commit", "--quiet", "-m", strings.TrimSpace(text))
		return git("rev-parse", "HEAD")
	}
	base := commit("Base.")
	heads := [3]string{commit("Revision A."), commit("Revision B. "), commit("Revision C.")}
	git("clone", "--quiet", "--bare", "--", repo, filepath.Join(remotes, "owner", "repo.git"))
	return base, heads, (&url.URL{Scheme: "file", Path: remotes}).String()
}

func (f *followFixture) connect(t *testing.T) (*sdk.ClientSession, *exec.Cmd) {
	t.Helper()
	return reviewProcess(t, f.server.URL, f.database, f.gitOrigin)
}

func (f *followFixture) read(t *testing.T, session *sdk.ClientSession, revision string) wire.ReviewWork {
	t.Helper()
	return reviewRequest(t, session, f.contracts, f.contracts.ReviewWork, "read-review", wire.ReviewWorkInput{
		IssueInput: wire.IssueInput{Repository: "owner/repo", Number: 42}, Revision: revision,
	})
}

func (f *followFixture) follow(t *testing.T, session *sdk.ClientSession) wire.ReviewWork {
	t.Helper()
	pr := wire.ReviewWorkInput{IssueInput: wire.IssueInput{Repository: "owner/repo", Number: 42}}
	work := reviewRequest(t, session, f.contracts, f.contracts.UpdateReviewWork, "inspect-original", wire.ReviewWorkChange{ReviewWorkInput: pr})
	pr.Revision = work.Latest
	for i, finding := range []struct {
		path     string
		decision wire.FindingDisposition
		reason   string
	}{
		{"main.go", wire.FindingAccepted, "Inspected the change at A."},
		{"steady.txt", wire.FindingDeferred, "Revisit the reference."},
	} {
		work = reviewRequest(t, session, f.contracts, f.contracts.UpdateReviewWork, "finding-"+finding.path, wire.ReviewWorkChange{
			ReviewWorkInput: pr, ExpectedVersion: work.Version,
			Finding: &wire.ReviewFindingInput{Path: finding.path, Side: wire.Right, Line: 1, Summary: finding.reason},
		})
		work = reviewRequest(t, session, f.contracts, f.contracts.UpdateReviewWork, "decision-"+finding.path, wire.ReviewWorkChange{
			ReviewWorkInput: pr, ExpectedVersion: work.Version,
			Decision: &wire.FindingDecisionInput{Finding: work.Findings[i].ID, Disposition: finding.decision, Reason: finding.reason},
		})
	}
	work = reviewRequest(t, session, f.contracts, f.contracts.UpdateReviewWork, "follow-source", wire.ReviewWorkChange{
		ReviewWorkInput: pr, ExpectedVersion: work.Version, Follow: new(true),
	})
	if !work.Following || work.Selected == nil || work.Selected.HeadSHA != f.source.heads[0] {
		t.Fatalf("original review was not followed: %+v", work)
	}
	return work
}

func (f *followFixture) waitNotice(t *testing.T) {
	t.Helper()
	select {
	case <-f.notices:
	case <-time.After(15 * time.Second):
		t.Fatal("no committed review change was announced")
	}
}

func (f *followFixture) checkB(t *testing.T, work, original wire.ReviewWork) {
	t.Helper()
	if work.Selected == nil || work.Selected.HeadSHA != f.source.heads[1] {
		t.Fatalf("wrong reviewed head: %+v", work)
	}
	recheck := work.Selected.Recheck
	if recheck == nil || recheck.Status != wire.ReviewRecheckIssues || recheck.CheckedHead != f.source.heads[1] ||
		recheck.PreviousRevision != original.Latest || recheck.CompletedAt == nil ||
		!strings.Contains(recheck.Diagnostic, "main.go:3: trailing whitespace.") {
		t.Fatalf("captured worktree was not verified at B: %+v", recheck)
	}
	want := []wire.FindingRecheck{
		{Finding: original.Findings[0].ID, Status: wire.FindingFileChanged},
		{Finding: original.Findings[1].ID, Status: wire.FindingFileUnchanged},
	}
	if !reflect.DeepEqual(recheck.Findings, want) || len(work.Findings) != 0 || !reflect.DeepEqual(work.NeedsRecheck, original.Findings) {
		t.Fatalf("recheck mixed file evidence and human decisions: recheck=%+v work=%+v", recheck, work)
	}
	if len(work.Checks) != 1 || work.Checks[0].HeadSHA != f.source.heads[1] || work.Checks[0].ID != 11 || !work.ChecksPartial ||
		len(work.Discussion) != 1 || work.Discussion[0].ReceivedAtRevision != work.Selected.ID {
		t.Fatalf("recheck mixed CI or duplicated discussion: %+v", work)
	}
}

func TestFollowedReviewChecksExactCommitOnceOnRepeatedDelivery(t *testing.T) {
	f := newFollowFixture(t)
	session, _ := f.connect(t)
	original := f.follow(t, session)
	unfollowed := reviewRequest(t, session, f.contracts, f.contracts.UpdateReviewWork, "inspect-unfollowed", wire.ReviewWorkChange{
		ReviewWorkInput: wire.ReviewWorkInput{IssueInput: wire.IssueInput{Repository: "owner/repo", Number: 43}},
	})
	if unfollowed.Following {
		t.Fatal("inspecting another PR enabled follow")
	}
	f.source.headIndex.Store(1)
	f.waitNotice(t)
	f.waitNotice(t)
	checked := f.read(t, session, "")
	f.checkB(t, checked, original)
	for range len(f.source.polled) {
		<-f.source.polled
	}
	deadline := time.After(15 * time.Second)
	for range 3 {
		select {
		case number := <-f.source.polled:
			if number != 42 {
				t.Fatalf("background followed PR %d without permission", number)
			}
		case <-deadline:
			t.Fatal("repeated upstream head observations were not delivered")
		}
	}
	repeated := f.read(t, session, "")
	if repeated.Version != checked.Version || len(repeated.Versions) != 2 ||
		!reflect.DeepEqual(repeated.Selected.Recheck, checked.Selected.Recheck) ||
		f.source.checkReads.Load() != 1 || f.source.reviewCount.Load() != 0 {
		t.Fatalf("duplicate head caused another review effect: first=%+v repeated=%+v", checked, repeated)
	}
	history := f.read(t, session, original.Latest)
	if !reflect.DeepEqual(history.Findings, original.Findings) {
		t.Fatalf("revision A lost its decisions: %+v", history.Findings)
	}
	paths, err := os.ReadDir(filepath.Join(filepath.Dir(f.database), "review-worktrees"))
	if err != nil || len(paths) != 0 {
		t.Fatalf("settled review left a scratch worktree: %v %v", paths, err)
	}
}

func TestFollowedReviewRestartsCapturedCommitBeforeNewerHead(t *testing.T) {
	f := newFollowFixture(t)
	session, process := f.connect(t)
	original := f.follow(t, session)
	f.hold.Store(true)
	f.source.headIndex.Store(1)
	f.waitNotice(t)
	pending := f.read(t, session, "")
	if pending.Selected == nil || pending.Selected.HeadSHA != f.source.heads[1] ||
		pending.Selected.Recheck == nil || pending.Selected.Recheck.Status != wire.ReviewRecheckPending {
		t.Fatalf("revision B was not durably scheduled: %+v", pending)
	}
	if err := process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = session.Close()
	f.source.headIndex.Store(2)
	f.hold.Store(false)
	session, _ = f.connect(t)
	f.waitNotice(t)
	recovered := f.read(t, session, pending.Latest)
	f.checkB(t, recovered, original)
	if recovered.CurrentHead != f.source.heads[2] {
		t.Fatalf("test did not advance the upstream head before recovery: %+v", recovered)
	}
	f.waitNotice(t)
	f.waitNotice(t)
	latest := f.read(t, session, "")
	if latest.Selected == nil || latest.Selected.HeadSHA != f.source.heads[2] || latest.Selected.Recheck == nil ||
		latest.Selected.Recheck.CheckedHead != f.source.heads[2] || latest.Selected.Recheck.Status != wire.ReviewRecheckClean ||
		latest.Selected.Recheck.PreviousRevision != pending.Latest || len(latest.Versions) != 3 ||
		len(latest.Discussion) != 1 || f.source.checkReads.Load() != 1 || f.source.reviewCount.Load() != 0 {
		t.Fatalf("recovery failed to continue to C independently: %+v", latest)
	}
}

func TestReviewPublicationRejectsNewHeadAndRecoversOriginalReceipt(t *testing.T) {
	f := newFollowFixture(t)
	session, _ := f.connect(t)
	f.follow(t, session)
	f.source.headIndex.Store(1)
	f.waitNotice(t)
	f.waitNotice(t)
	checked := f.read(t, session, "")
	publish := wire.SubmitReviewInput{
		IssueInput: checked.IssueInput, CommitID: wire.CommitSHA(f.source.heads[1]),
		Event: wire.CommentReview, Body: new("Checked this revision."),
	}
	first := reviewRequest(t, session, f.contracts, f.contracts.SubmitReview, "published", publish)
	if first.ID != 40 || first.CommitID == nil || *first.CommitID != f.source.heads[1] {
		t.Fatalf("publication receipt was not pinned to B: %+v", first)
	}
	draft := publish
	draft.Body = new("Still waiting to publish this B draft.")
	f.source.headIndex.Store(2)
	_, err := reviewResult(t, session, f.contracts, f.contracts.SubmitReview, "stale-draft", draft)
	refusal, known := errors.AsType[*framecall.Refusal](err)
	if !known || refusal.Code != "conflict" || f.source.reviewCount.Load() != 1 {
		t.Fatalf("stale draft reached GitHub or returned the wrong outcome: writes=%d error=%v", f.source.reviewCount.Load(), err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	session, _ = f.connect(t)
	replay := reviewRequest(t, session, f.contracts, f.contracts.SubmitReview, "published", publish)
	input, err := framevalue.EncodeJSON(publish)
	if err != nil {
		t.Fatal(err)
	}
	raw, found, err := githubBinding(t, session, f.contracts, f.contracts.SubmitReview.Name()).ReadReceipt(t.Context(), "review-published", input)
	var receipt wire.Review
	if err != nil || !found || raw.Decode(&receipt) != nil || !reflect.DeepEqual(receipt, first) ||
		!reflect.DeepEqual(replay, first) || f.source.reviewCount.Load() != 1 {
		t.Fatalf("original receipt did not survive the new head and restart: found=%v writes=%d replay=%+v err=%v",
			found, f.source.reviewCount.Load(), replay, err)
	}
}
