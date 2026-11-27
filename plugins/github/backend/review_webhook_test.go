package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/plugins/github/internal/wire"
)

func TestReviewWebhookResumesCapturedHeadWithProjectChecks(t *testing.T) {
	f := newFollowFixture(t)
	const secret = "local-review-webhook-fixture"
	address := f.enableWebhook(t, secret, `{
		"owner/repo":{
			"environment":"fixture-v1",
			"checks":[
				{"name":"workspace","command":"/bin/sh check.sh","timeoutSeconds":5},
				{"name":"failure","command":"/usr/bin/grep -F Revision main.go && exit 7","timeoutSeconds":5},
				{"name":"timeout","command":"exec /bin/sleep 45","timeoutSeconds":1}
			]
		}
	}`)
	session, process := f.connect(t)
	original := f.follow(t, session)
	f.hold.Store(true)
	f.deliver(t, address, "wrong-signing-secret", 42, http.StatusUnauthorized)
	f.deliver(t, address, secret, 43, http.StatusNoContent)
	f.deliver(t, address, secret, 42, http.StatusAccepted)
	f.waitNotice(t)
	captured := f.read(t, session, f.source.base+":"+f.source.heads[1])
	if captured.Latest != original.Latest || captured.CurrentHead != f.source.heads[0] ||
		captured.Selected.Recheck.Status != wire.ReviewRecheckPending ||
		captured.Selected.Recheck.Project == nil || len(captured.Selected.Recheck.Project.Checks) != 3 {
		t.Fatalf("webhook did not durably capture B without moving the current head: %+v", captured)
	}
	f.deliver(t, address, secret, 42, http.StatusNoContent)
	if err := process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = session.Close()
	f.source.headIndex.Store(2)
	f.hold.Store(false)
	session, _ = f.connect(t)
	for range 4 {
		f.waitNotice(t)
	}
	checked := f.read(t, session, captured.Selected.ID)
	result := checked.Selected.Recheck
	if checked.CurrentHead != f.source.heads[2] || result.CheckedHead != f.source.heads[1] ||
		result.Status != wire.ReviewRecheckIssues || len(result.Findings) != 2 ||
		!strings.Contains(result.Findings[0].Patch, "+// Revision B.") ||
		len(checked.Checks) != 1 || checked.Checks[0].HeadSHA != f.source.heads[1] {
		t.Fatalf("restarted webhook mixed B evidence with the current head: %+v", checked)
	}
	project := result.Project
	if project == nil || project.Environment != "fixture-v1" || project.Runner == "" ||
		project.PolicyDigest != captured.Selected.Recheck.Project.PolicyDigest || len(project.Checks) != 3 {
		t.Fatalf("project check receipt changed policy: %+v", project)
	}
	if runtime.GOOS == "darwin" {
		passed, failed, timedOut := project.Checks[0], project.Checks[1], project.Checks[2]
		if passed.Status != wire.ProjectCheckPassed || !strings.Contains(passed.Log, "Revision B.") ||
			failed.Status != wire.ProjectCheckFailed || failed.ExitCode == nil || *failed.ExitCode != 7 || !strings.Contains(failed.Log, "Revision B.") ||
			timedOut.Status != wire.ProjectCheckTimedOut || timedOut.ExitCode != nil {
			t.Fatalf("project checks did not capture passing, failing and timed out measures independently: %+v", project.Checks)
		}
	} else {
		for _, check := range project.Checks {
			if check.Status != wire.ProjectCheckUnavailable || check.ExitCode != nil {
				t.Fatalf("unsupported platform executed a project check: %+v", check)
			}
		}
	}
	latest := f.read(t, session, "")
	if latest.Selected.HeadSHA != f.source.heads[2] || latest.Selected.Recheck.CheckedHead != f.source.heads[2] ||
		len(latest.Selected.Recheck.Findings) != 2 || len(latest.Versions) != 3 {
		t.Fatalf("poll reconciliation did not independently verify C: %+v", latest)
	}
	f.deliver(t, address, secret, 42, http.StatusNoContent)
	if repeated := f.read(t, session, captured.Selected.ID); repeated.Version != latest.Version ||
		!reflect.DeepEqual(repeated.Selected.Recheck, result) || f.source.checkReads.Load() != 1 || f.source.reviewCount.Load() != 0 {
		t.Fatalf("redelivery changed a settled recheck: %+v", repeated)
	}
	decided := reviewRequest(t, session, f.contracts, f.contracts.UpdateReviewWork, "resolve-at-b", wire.ReviewWorkChange{
		ReviewWorkInput: wire.ReviewWorkInput{IssueInput: checked.IssueInput, Revision: checked.Selected.ID},
		ExpectedVersion: latest.Version,
		Decision:        &wire.FindingDecisionInput{Finding: result.Findings[0].Finding, Disposition: wire.FindingResolved, Reason: "Verified the change at B."},
	})
	if decisions := decided.Selected.Recheck.Findings[0].Decisions; len(decisions) != 1 || decisions[0].Disposition != wire.FindingResolved {
		t.Fatalf("did not record a separate human recheck decision: %+v", decided)
	}
	if historical := f.read(t, session, original.Latest); !reflect.DeepEqual(historical.Findings, original.Findings) {
		t.Fatalf("deciding B rewrote the original findings: %+v", historical)
	}
	if paths, err := os.ReadDir(filepath.Join(filepath.Dir(f.database), "review-worktrees")); err != nil || len(paths) != 0 {
		t.Fatalf("settled checks left scratch state: %v %v", paths, err)
	}
}

func (f *followFixture) enableWebhook(t *testing.T, secret, policies string) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	f.env = []string{reviewWebhookPort + "=" + port, reviewWebhookSecret + "=" + secret, projectChecksVariable + "=" + policies}
	return "http://" + address + reviewWebhookPath
}

func (f *followFixture) deliver(t *testing.T, address, secret string, number, status int) {
	t.Helper()
	raw := fmt.Sprintf(`{"action":"synchronize","number":%d,"before":%q,"after":%q,
		"repository":{"full_name":"owner/repo"},
		"pull_request":{"number":%d,"title":"Captured revision B",
			"base":{"ref":"main","sha":%q,"repo":{"full_name":"owner/repo"}},
			"head":{"ref":"topic","sha":%q,"repo":{"full_name":"owner/repo"}}}}`,
		number, f.source.heads[0], f.source.heads[1], number, f.source.base, f.source.heads[1])
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(raw))
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, address, strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", "pull_request")
	request.Header.Set("X-GitHub-Delivery", "fixture-delivery")
	request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("webhook status = %d, want %d", response.StatusCode, status)
	}
}
