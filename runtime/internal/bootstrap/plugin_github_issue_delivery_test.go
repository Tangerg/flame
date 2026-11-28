package bootstrap

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

const (
	githubIssueFixtureRepo   = "owner/repo"
	githubIssueFixtureToken  = "test-github-issue-token"
	githubIssueFixtureSecret = "test-issue-signing-secret"
	githubIssueFixtureTime   = "2026-11-28T00:00:00Z"
)

func TestGitHubIssueDeliveryRecoversAndPublishesExactEvidence(t *testing.T) {
	fixture, origin := newIssueDeliveryFixture(t)
	api := httptest.NewServer(fixture)
	t.Cleanup(api.Close)
	source := githubIssueFixturePackage(t, api.URL, origin)
	port := githubIssueFixturePort(t)
	cfg := poMCPConfig(t)
	cfg.approvalMode = approval.ModeYolo
	var modelCalls atomic.Int32
	model, err := chatclient.New(delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		var names []string
		for _, tool := range request.Tools {
			names = append(names, tool.Name)
		}
		slices.Sort(names)
		if !slices.Equal(names, []string{"glob", "grep", "read"}) {
			t.Errorf("verification Run can invoke unexpected tools: %v", names)
		}
		modelCalls.Add(1)
		return completedTextResponse("Inspected the captured checkout."), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(model)
	opening := make(chan protocol.StartRunResponse, 1)
	var starts atomic.Int32
	var allowRecovery atomic.Bool
	decorate := loseFirstIssueOpening(t, opening, &starts, &allowRecovery)
	r := poMCPOpenWithCallbackRPC(t, cfg, decorate)
	project := r.must(delivery.Name("projects.create"), protocol.CreateProjectRequest{Name: "Issue delivery"}, "create-project").(*protocol.Project)
	activity := func() *protocol.ProjectBackgroundActivity {
		return r.must(delivery.ProjectsGetBackground, protocol.GetProjectRequest{ProjectID: project.ID}, "").(*protocol.ProjectBackgroundActivity)
	}
	policy := activity().Policy
	policy.Paused = true
	setPause := func(paused bool, key string) {
		policy.Paused = paused
		policy = *r.must(delivery.ProjectsSetBackground, protocol.SetProjectBackgroundPolicyRequest{ProjectID: project.ID, Policy: policy}, key).(*protocol.ProjectBackgroundPolicy)
	}
	setPause(true, "pause-issue-admission")
	installed := r.installAndInspect(source, "install-issue-delivery")
	if len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("GitHub Issue fixture release diagnostics: %+v", installed.Selected.Diagnostics)
	}
	values := map[string]protocol.PluginValueChange{}
	for id, value := range map[string]string{
		"token":          githubIssueFixtureToken,
		"issue-delivery": fmt.Sprintf(`{"repository":%q,"projectId":%q,"branch":"main","label":"flame-delivery","provider":"anthropic","model":"claude-test"}`, githubIssueFixtureRepo, project.ID),
		"review-checks":  `{"owner/repo":{"environment":"loopback-fixture","checks":[{"name":"Captured tree","command":"/usr/bin/grep -qx captured expected.txt","timeoutSeconds":10}]}}`,
		"webhook-port":   port,
		"webhook-secret": githubIssueFixtureSecret,
	} {
		values[id] = protocol.PluginValueChange{Type: protocol.PluginValueSet, Value: &value}
	}
	r.must(delivery.PluginsConfigure, protocol.ConfigurePluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, ValueChanges: values}, "configure-issue-delivery")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-issue-delivery")
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "github"}
	waitForConnector := func() {
		r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) > 0 })
	}
	waitForConnector()

	contract, err := plugincontract.ParseCapability([]byte(testsupport.GitHubCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, value any, key string) *protocol.CallPluginResponse {
		t.Helper()
		input, err := framevalue.EncodeJSON(value)
		if err != nil {
			t.Fatal(err)
		}
		return r.must(delivery.PluginsCall, protocol.CallPluginRequest{Route: protocol.PluginCallRoute{
			InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "github-operations",
			Contract: contract.Identity().String(), Method: method, Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
		}, Input: input.String()}, key).(*protocol.CallPluginResponse)
	}
	list := func() testsupport.GitHubIssueDeliveryPage {
		return decodeIssueDelivery[testsupport.GitHubIssueDeliveryPage](t, call(testsupport.GitHubIssueDeliveriesV1Method, testsupport.GitHubIssueDeliveriesInput{ProjectId: project.ID}, ""))
	}
	publish := func(row *testsupport.GitHubIssueDelivery, digest, key string) *protocol.CallPluginResponse {
		return call(testsupport.GitHubPublishIssueDeliveryV1Method, testsupport.GitHubPublishIssueDeliveryInput{Id: row.Id, Digest: digest}, key)
	}
	wait := func(number, status string) *testsupport.GitHubIssueDelivery {
		return waitIssueDelivery(t, list, number, status)
	}

	fixture.webhook(t, port, "bad", "invalid", http.StatusUnauthorized)
	fixture.webhook(t, port, "first", githubIssueFixtureSecret, http.StatusAccepted)
	fixture.webhook(t, port, "first", githubIssueFixtureSecret, http.StatusNoContent)
	fixture.webhook(t, port, "different-delivery", githubIssueFixtureSecret, http.StatusNoContent)
	queued := wait("41", "queued")
	if len(list().Items) != 1 || queued.RunId != nil || activity().Admitted != 0 {
		t.Fatalf("duplicate delivery or paused project created an extra admission: %+v", queued)
	}
	t.Log("duplicate signed deliveries retained one queue row under the Project pause")
	setPause(false, "resume-issue-admission")
	var accepted protocol.StartRunResponse
	select {
	case accepted = <-opening:
	case <-time.After(25 * time.Second):
		t.Fatalf("did not receive the committed Run whose response was discarded; last queue: %+v", list().Items)
	}
	if before := activity(); before.Admitted != 1 || len(before.Recent) != 1 || before.Recent[0].RunID != accepted.RunID {
		t.Fatalf("the dropped opening did not leave one original admission: %+v", before)
	}
	waitForIssueRun(t, r, accepted.RunID)
	if pending := wait("41", "queued"); pending.RunId != nil {
		t.Fatal("the fixture did not withhold the private acceptance before restart")
	}
	fixture.reconcileSecond.Store(true)
	r.close()
	reopenPluginStores(t, &cfg)
	allowRecovery.Store(true)
	r = poMCPOpenWithCallbackRPC(t, cfg, decorate)
	waitForConnector()
	phase := "checking"
	if goruntime.GOOS != "darwin" {
		phase = "blocked"
	}
	first, second := wait("41", phase), wait("42", phase)
	if first.Id != queued.Id || first.RunId == nil || *first.RunId != accepted.RunID || starts.Load() != 2 || activity().Admitted != 2 {
		t.Fatalf("lost acknowledgement or restart created a second Run for the same Issue: starts=%d activity=%+v", starts.Load(), activity())
	}
	if secondReads := fixture.secondPage.Load(); secondReads == 0 {
		t.Fatal("the Issue without a webhook was not recovered from the next GitHub page")
	}
	if first.Workspace == nil || second.Workspace == nil || *first.Workspace == *second.Workspace || modelCalls.Load() != 2 {
		t.Fatalf("the Issues did not each get an independent Run and worktree: first=%+v second=%+v calls=%d", first, second, modelCalls.Load())
	}
	for _, issue := range []*testsupport.GitHubIssueDelivery{first, second} {
		if issue.Head == nil || *issue.Head != fixture.head || issueGit(t, *issue.Workspace, "rev-parse", "HEAD") != fixture.head || issueGit(t, *issue.Workspace, "rev-parse", "--abbrev-ref", "HEAD") != "HEAD" {
			t.Fatalf("Issue %s did not retain its exact detached SHA: %+v", issue.Number.String(), issue)
		}
	}
	t.Log("restart recovered the original Run after a lost acknowledgement; a missed Issue was admitted from page two")
	if goruntime.GOOS != "darwin" {
		if fixture.posts.Load() != 0 || first.Digest != nil || second.Digest != nil {
			t.Fatal("an unsupported local sandbox produced an approvable report")
		}
		return
	}

	if first.Ci.Status != "incomplete" || second.Ci.Status != "incomplete" || first.Draft != nil || first.Digest != nil || fixture.posts.Load() != 0 {
		t.Fatalf("a partial CI listing was treated as a complete verdict: %+v", first)
	}
	fixture.completeChecks.Store(true)
	fixture.webhook(t, port, "refresh-complete", githubIssueFixtureSecret, http.StatusNoContent)
	first, second = wait("41", "ready"), wait("42", "ready")
	fixture.completeChecks.Store(false)
	assertIssueDeliveryRefused(t, publish(first, *first.Digest, "incomplete-before-publish"), "conflict", "complete, passing GitHub checks")
	if fixture.posts.Load() != 0 || first.ApprovedAt != nil || first.Receipt != nil {
		t.Fatal("incomplete CI or an unapproved draft triggered publication")
	}
	t.Log("incomplete CI blocked a draft and invalidated a previously complete approval packet at publish time")
	fixture.completeChecks.Store(true)
	published := decodeIssueDelivery[testsupport.GitHubPublishIssueDeliveryV1OutputWireIssueDeliveryReceipt](t, publish(first, *first.Digest, "approve-original"))
	if published.Head != fixture.head || published.Digest != *first.Digest || published.Comment == nil || published.Comment.Body != *first.Draft || !strings.Contains(published.Comment.Body, accepted.RunID) || fixture.posts.Load() != 1 {
		t.Fatalf("publication did not retain the approved report and upstream receipt: %+v", published)
	}
	assertIssueDeliveryRefused(t, publish(first, *first.Digest, "another-approval"), "conflict", "original publication command")
	fixture.advanceHeadAfterRead.Store(true)
	assertIssueDeliveryRefused(t, publish(second, *second.Digest, "stale-head"), "conflict", "branch head changed")
	refused := wait("42", "publicationRefused")
	if refused.ApprovedAt == nil || refused.Receipt != nil || fixture.posts.Load() != 1 {
		t.Fatalf("head drift after approval produced a comment or lost the original decision: %+v", refused)
	}
	t.Log("a branch change between approval and submission blocked the post and retained the refusal")
	r = reopenPluginRuntime(t, r, &cfg)
	waitForConnector()
	restored := wait("41", "published")
	wait("42", "publicationRefused")
	if restored.Receipt == nil || restored.Receipt.Comment == nil || restored.ApprovedAt == nil || restored.Receipt.Head != fixture.head || restored.Receipt.Comment.Id.String() != published.Comment.Id.String() || restored.Workspace == nil || *restored.Workspace != *first.Workspace {
		t.Fatalf("the original approval, worktree or receipt vanished on restart: %+v", restored)
	}
	replay := decodeIssueDelivery[testsupport.GitHubPublishIssueDeliveryV1OutputWireIssueDeliveryReceipt](t, publish(first, *first.Digest, "approve-original"))
	if replay.Comment == nil || replay.Comment.Id.String() != published.Comment.Id.String() || fixture.posts.Load() != 1 {
		t.Fatalf("recovering a publication changed the original upstream effect: %+v", replay)
	}
	t.Log("the approved body, original comment ID and retained workspace survived restart and head drift")

	fixture.revokeLabel.Store(true)
	fixture.webhook(t, port, "changed-label", githubIssueFixtureSecret, http.StatusNoContent)
	if page := list(); len(page.Items) != 1 || page.Items[0].Id != first.Id {
		t.Fatalf("label withdrawal exposed verification data: %+v", page.Items)
	}
	assertIssueDeliveryRefused(t, publish(second, *second.Digest, "revoked-label"), "not_found", "no longer authorized")
	fixture.revokeRepository.Store(true)
	if page := list(); len(page.Items) != 0 {
		t.Fatalf("lost repository access exposed stored evidence or receipt: %+v", page.Items)
	}
	assertIssueDeliveryRefused(t, publish(second, *second.Digest, "revoked-repository"), "forbidden", "")
	if fixture.posts.Load() != 1 || activity().Admitted != 2 {
		t.Fatal("revocation, refresh or restart caused a new external write or admission")
	}
	t.Log("label removal and repository permission loss hid evidence and refused publication")
}

func waitForIssueRun(t *testing.T, r *poMCPRuntime, id string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: id}, "").(*protocol.RunRef)
		if run.Status == protocol.RunStatusFinished {
			if run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
				t.Fatalf("the original accepted Run did not complete: %+v", run.Outcome)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the original accepted Run did not finish: %+v", run)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func loseFirstIssueOpening(t *testing.T, opening chan<- protocol.StartRunResponse, starts *atomic.Int32, allowRecovery *atomic.Bool) func(http.Handler) http.Handler {
	return func(rpc http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Error(err)
				return
			}
			request.Body = io.NopCloser(bytes.NewReader(body))
			var envelope struct {
				Method string `json:"method"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Error(err)
				return
			}
			if envelope.Method == "runs.start" && starts.Add(1) == 1 {
				rpc.ServeHTTP(lostOpeningWriter{ResponseWriter: w, opening: opening}, request)
				return
			}
			if envelope.Method == "runs.getAcceptance" && starts.Load() > 0 && !allowRecovery.Load() {
				http.Error(w, "fixture withholding acceptance until restart", http.StatusServiceUnavailable)
				return
			}
			rpc.ServeHTTP(w, request)
		})
	}
}

func decodeIssueDelivery[T any](t *testing.T, response *protocol.CallPluginResponse) T {
	t.Helper()
	if response.Failure != nil {
		t.Fatalf("Issue operation failed: %+v", response.Failure)
	}
	var value T
	if err := json.Unmarshal([]byte(response.Output), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func assertIssueDeliveryRefused(t *testing.T, response *protocol.CallPluginResponse, code, message string) {
	t.Helper()
	if response.Failure == nil || response.Failure.Code != code || !strings.Contains(response.Failure.Message, message) {
		t.Fatalf("expected %s containing %q, got %+v", code, message, response)
	}
}

func waitIssueDelivery(t *testing.T, list func() testsupport.GitHubIssueDeliveryPage, number, status string) *testsupport.GitHubIssueDelivery {
	t.Helper()
	deadline := time.After(30 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	var observed string
	for {
		for _, item := range list().Items {
			if item.Number.String() != number {
				continue
			}
			observed = item.Status
			if item.Reason != nil {
				observed += ": " + *item.Reason
			}
			if item.Status == status && (status != "checking" || item.Ci != nil) {
				return item
			}
		}
		select {
		case <-deadline:
			t.Fatalf("Issue #%s never reached %s: %s", number, status, observed)
		case <-tick.C:
		}
	}
}

func githubIssueFixturePackage(t *testing.T, endpoint, origin string) string {
	t.Helper()
	_, file, _, _ := goruntime.Caller(0)
	plugins := filepath.Join(filepath.Dir(file), "../../../plugins")
	source := filepath.Join(t.TempDir(), "github")
	if err := os.CopyFS(source, os.DirFS(filepath.Join(plugins, "github", "package"))); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(source, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), filepath.Join(goruntime.GOROOT(), "bin", "go"), "test", "-c", "-o", filepath.Join(bin, "github"), "./github/backend")
	command.Dir = plugins
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build GitHub fixture: %v\n%s", err, output)
	}
	transport := fmt.Sprintf(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"github":{"type":"stdio","command":"./bin/github","env":{"FLAME_TEST_GITHUB_ISSUE_ENDPOINT":%q,"FLAME_TEST_GITHUB_ISSUE_GIT_ORIGIN":%q}}}}`, endpoint, origin)
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), []byte(transport), 0600); err != nil {
		t.Fatal(err)
	}
	authorPluginPackage(t, source)
	return source
}

type issueDeliveryFixture struct {
	t                                                                           *testing.T
	head, next                                                                  string
	reconcileSecond, completeChecks, headChanged, revokeLabel, revokeRepository atomic.Bool
	secondPage, posts                                                           atomic.Int32
	advanceHeadAfterRead                                                        atomic.Bool
}

func newIssueDeliveryFixture(t *testing.T) (*issueDeliveryFixture, string) {
	t.Helper()
	workspace, origin := t.TempDir(), t.TempDir()
	issueGit(t, workspace, "init", "--initial-branch=main")
	write := func(body string) {
		if err := os.WriteFile(filepath.Join(workspace, "expected.txt"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		issueGit(t, workspace, "add", "expected.txt")
		issueGit(t, workspace, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-m", strings.TrimSpace(body))
	}
	write("captured\n")
	head := issueGit(t, workspace, "rev-parse", "HEAD")
	write("moved\n")
	next := issueGit(t, workspace, "rev-parse", "HEAD")
	destination := filepath.Join(origin, githubIssueFixtureRepo+".git")
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		t.Fatal(err)
	}
	issueGit(t, origin, "clone", "--bare", workspace, destination)
	return &issueDeliveryFixture{t: t, head: head, next: next}, "file://" + origin
}

func issueGit(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", args...)
	command.Dir = cwd
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (f *issueDeliveryFixture) issue(number int, labeled bool) string {
	labels := "[]"
	if labeled {
		labels = `[{"name":"flame-delivery"}]`
	}
	return fmt.Sprintf(`{"id":%d,"number":%d,"title":"Verify issue %d","body":"Inspect the committed fixture.","state":"open","html_url":"https://github.com/owner/repo/issues/%d","updated_at":%q,"labels":%s,"assignees":[]}`, 1000+number, number, number, number, githubIssueFixtureTime, labels)
}

func (f *issueDeliveryFixture) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if request.Header.Get("Authorization") != "Bearer "+githubIssueFixtureToken {
		f.t.Error("GitHub fixture request omitted its installation credential")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch request.Method + " " + request.URL.Path {
	case "GET /repos/owner/repo":
		if f.revokeRepository.Load() {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Access revoked"}`)
			return
		}
		fmt.Fprint(w, `{"id":901,"full_name":"owner/repo"}`)
	case "GET /repos/owner/repo/branches/main":
		head := f.head
		if f.headChanged.Load() {
			head = f.next
		}
		if f.advanceHeadAfterRead.Swap(false) {
			f.headChanged.Store(true)
		}
		fmt.Fprintf(w, `{"name":"main","commit":{"sha":%q}}`, head)
	case "GET /repos/owner/repo/issues":
		if !f.reconcileSecond.Load() {
			fmt.Fprint(w, `[]`)
			return
		}
		if request.URL.Query().Get("page") == "2" {
			f.secondPage.Add(1)
			if f.revokeLabel.Load() {
				fmt.Fprint(w, `[]`)
			} else {
				fmt.Fprint(w, "["+f.issue(42, true)+"]")
			}
			return
		}
		w.Header().Set("Link", `<https://api.github.com/repos/owner/repo/issues?page=2>; rel="next"`)
		fmt.Fprint(w, "["+f.issue(41, true)+"]")
	case "GET /repos/owner/repo/issues/41":
		fmt.Fprint(w, f.issue(41, true))
	case "GET /repos/owner/repo/issues/42":
		fmt.Fprint(w, f.issue(42, !f.revokeLabel.Load()))
	case "GET /repos/owner/repo/commits/" + f.head + "/check-runs":
		id := 11
		if f.completeChecks.Load() {
			if request.URL.Query().Get("page") == "2" {
				id = 12
			} else {
				w.Header().Set("Link", `<https://api.github.com/repos/owner/repo/commits/`+f.head+`/check-runs?page=2>; rel="next"`)
			}
		}
		fmt.Fprintf(w, `{"total_count":2,"check_runs":[{"id":%d,"name":"CI %d","head_sha":%q,"status":"completed","conclusion":"success","output":{"annotations_count":0}}]}`, id, id, f.head)
	case "POST /repos/owner/repo/issues/41/comments", "POST /repos/owner/repo/issues/42/comments":
		f.posts.Add(1)
		var comment struct {
			Body string `json:"body"`
		}
		if err := json.UnmarshalRead(request.Body, &comment); err != nil {
			f.t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"id":701,"html_url":"https://github.com/owner/repo/issues/41#issuecomment-701","body":%q,"user":{"login":"test-app"},"created_at":%q,"updated_at":%q}`, comment.Body, githubIssueFixtureTime, githubIssueFixtureTime)
	default:
		f.t.Errorf("unexpected GitHub request: %s %s", request.Method, request.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func githubIssueFixturePort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func (f *issueDeliveryFixture) webhook(t *testing.T, port, id, secret string, want int) {
	t.Helper()
	body := fmt.Sprintf(`{"action":"labeled","repository":{"id":901,"full_name":"owner/repo"},"issue":%s}`, f.issue(41, true))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1:"+port+"/github/review-webhook", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%x", mac.Sum(nil)))
	request.Header.Set("X-GitHub-Event", "issues")
	request.Header.Set("X-GitHub-Delivery", id)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("webhook status = %d, want %d", response.StatusCode, want)
	}
}
