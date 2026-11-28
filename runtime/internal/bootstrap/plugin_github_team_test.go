package bootstrap

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	goruntime "runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	framecall "github.com/Tangerg/frame/call"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestGitHubTeamRestrictsOriginalDeliveryAndRetainsSignedWorkSources(t *testing.T) {
	fixture, origin := newIssueDeliveryFixture(t)
	fixture.completeChecks.Store(true)
	api := httptest.NewServer(fixture)
	t.Cleanup(api.Close)
	source := githubIssueFixturePackage(t, api.URL, origin)
	port := githubIssueFixturePort(t)
	public := "http://127.0.0.1:" + port
	const linearSecret = "fixture-linear-signing-secret"
	const slackSecret = "fixture-slack-signing-secret"
	cfg := poMCPConfig(t)
	cfg.approvalMode = approval.ModeYolo
	model, err := chatclient.New(delegateRestartModel{chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return completedTextResponse("Read the original checkout."), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(model)
	r := poMCPOpen(t, cfg)
	project := r.must(delivery.Name("projects.create"), protocol.CreateProjectRequest{Name: "Original team project"}, "create-team-project").(*protocol.Project)
	other := r.must(delivery.Name("projects.create"), protocol.CreateProjectRequest{Name: "Independent project"}, "create-other-project").(*protocol.Project)
	installed := r.installAndInspect(source, "install-project-team")
	if len(installed.Selected.Diagnostics) != 0 {
		t.Fatalf("team package was rejected: %+v", installed.Selected.Diagnostics)
	}
	values := map[string]protocol.PluginValueChange{}
	for id, value := range map[string]string{
		"token":          githubIssueFixtureToken,
		"issue-delivery": fmt.Sprintf(`{"repository":%q,"projectId":%q,"branch":"main","label":"flame-delivery","provider":"anthropic","model":"claude-test"}`, githubIssueFixtureRepo, project.ID),
		"review-checks":  `{"owner/repo":{"environment":"loopback-fixture","checks":[{"name":"Exact file","command":"/usr/bin/grep -qx captured expected.txt","timeoutSeconds":10}]}}`,
		"webhook-port":   port, "webhook-secret": githubIssueFixtureSecret,
		"team-origin": public, "team-secret": "isolated-team-secret-for-the-test-fixture",
		"linear-secret": linearSecret, "slack-secret": slackSecret,
		"work-intake": fmt.Sprintf(`{"projectId":%q,"linearOrganization":"org-original","linearTeam":"team-original","slackTeam":"TTEAM","slackChannel":"CCODING","slackOrigin":"https://sample.slack.com"}`, project.ID),
	} {
		values[id] = protocol.PluginValueChange{Type: protocol.PluginValueSet, Value: &value}
	}
	r.must(delivery.PluginsConfigure, protocol.ConfigurePluginRequest{
		InstallationID: installed.ID, Digest: installed.Selected.Digest, ValueChanges: values,
	}, "configure-project-team")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-project-team")
	waitConnected := func() { waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected) }
	waitConnected()
	contract, err := plugincontract.ParseCapability([]byte(testsupport.GitHubCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, key string, value any) *protocol.CallPluginResponse {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return r.must(delivery.PluginsCall, protocol.CallPluginRequest{Route: protocol.PluginCallRoute{
			InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "github-operations",
			Contract: contract.Identity().String(), Method: "flame.github/" + method + ".v1",
			Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
		}, Input: string(raw)}, key).(*protocol.CallPluginResponse)
	}
	readBoard := func(projectID string) testsupport.GitHubTeamBoard {
		t.Helper()
		return decodeIssueDelivery[testsupport.GitHubTeamBoard](t, call("team-board", "", map[string]string{"projectId": projectID}))
	}
	client := &http.Client{Timeout: 8 * time.Second}
	request := func(method, path, body string, headers map[string]string, want int) []byte {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), method, public+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil || resp.StatusCode != want {
			t.Fatalf("%s %s: status=%d want=%d body=%q read=%v", method, path, resp.StatusCode, want, raw, err)
		}
		return raw
	}
	sign := func(secret, value string) string {
		digest := hmac.New(sha256.New, []byte(secret))
		_, _ = io.WriteString(digest, value)
		return fmt.Sprintf("%x", digest.Sum(nil))
	}
	linear := func(team string, stamp time.Time, secret string, status int) {
		body := fmt.Sprintf(`{"type":"Issue","action":"create","organizationId":"org-original","webhookTimestamp":%d,"url":"https://linear.app/example/issue/CODE-1","data":{"id":"issue-original","title":"Verify the original task","teamId":%q}}`, stamp.UnixMilli(), team)
		request(http.MethodPost, "/flame/intake/linear", body, map[string]string{"Linear-Signature": sign(secret, body)}, status)
	}
	slack := func(channel, body string, stamp time.Time, status int) {
		if body == "" {
			body = fmt.Sprintf(`{"type":"event_callback","team_id":"TTEAM","event_id":"EVORIGINAL","event":{"type":"app_mention","channel":%q,"ts":"1710000000.123456","text":"Inspect the original thread"}}`, channel)
		}
		timestamp := strconv.FormatInt(stamp.Unix(), 10)
		request(http.MethodPost, "/flame/intake/slack", body, map[string]string{
			"X-Slack-Request-Timestamp": timestamp, "X-Slack-Signature": "v0=" + sign(slackSecret, "v0:"+timestamp+":"+body),
		}, status)
	}
	now := time.Now()
	linear("team-original", now, "wrong-linear-secret", http.StatusUnauthorized)
	linear("team-original", now.Add(-2*time.Minute), linearSecret, http.StatusUnauthorized)
	linear("outside", now, linearSecret, http.StatusForbidden)
	linear("team-original", now, linearSecret, http.StatusOK)
	linear("team-original", now, linearSecret, http.StatusOK)
	slack("", `{"type":"url_verification","challenge":"flame-route"}`, now, http.StatusOK)
	slack("COTHER", "", now, http.StatusForbidden)
	slack("CCODING", "", now.Add(-6*time.Minute), http.StatusUnauthorized)
	slack("CCODING", "", now, http.StatusOK)
	slack("CCODING", "", now, http.StatusOK)
	initial := readBoard(project.ID)
	if !initial.Sharing || len(initial.Sources) != 2 ||
		initial.Sources[0].Url != "https://sample.slack.com/archives/CCODING/p1710000000123456" ||
		initial.Sources[1].Url != "https://linear.app/example/issue/CODE-1" {
		t.Fatalf("original work links or deduplication were lost: %+v", initial.Sources)
	}
	if outside := readBoard(other.ID); outside.Sharing || len(outside.Sources) != 0 {
		t.Fatalf("sources crossed into an unrelated Project: %+v", outside)
	}

	fixture.webhook(t, port, "team-original-work", githubIssueFixtureSecret, http.StatusAccepted)
	list := func() testsupport.GitHubIssueDeliveryPage {
		return decodeIssueDelivery[testsupport.GitHubIssueDeliveryPage](t, call("issue-deliveries", "", map[string]string{"projectId": project.ID}))
	}
	status := "ready"
	if goruntime.GOOS != "darwin" {
		status = "blocked"
	}
	row := waitIssueDelivery(t, list, "41", status)
	type membership struct{ ID, URL string }
	grant := func(member, role string, projectAccess, machineAccess bool) membership {
		t.Helper()
		value := decodeIssueDelivery[testsupport.GitHubTeamGrantV1OutputWireTeamGrant](t, call("team-grant", "grant-"+member, map[string]any{
			"projectId": project.ID, "deliveryId": row.Id, "member": member, "role": role,
			"projectAccess": projectAccess, "machineAccess": machineAccess,
		}))
		link := decodeIssueDelivery[testsupport.GitHubTeamInvite](t, call("team-invite", "", map[string]string{"projectId": project.ID, "id": value.Id}))
		return membership{ID: value.Id, URL: link.Url}
	}
	revoke := func(member membership) {
		t.Helper()
		decodeIssueDelivery[testsupport.GitHubTeamRevokeV1OutputWireTeamGrant](t, call("team-revoke", "revoke-"+member.ID, map[string]string{"id": member.ID, "projectId": project.ID}))
	}
	taskURL := "/flame/team"
	mobile := func(member membership, method, path, body string, want int) []byte {
		t.Helper()
		captured, err := url.Parse(member.URL)
		if err != nil || captured.Fragment == "" || captured.Path != taskURL {
			t.Fatalf("bad private invitation: %v", err)
		}
		return request(method, taskURL+path, body, map[string]string{
			"Authorization": "Bearer " + captured.Fragment, "Content-Type": "application/json",
		}, want)
	}
	raw := request(http.MethodGet, taskURL, "", nil, http.StatusOK)
	if !strings.Contains(string(raw), "Approve and publish to GitHub") || !strings.Contains(string(raw), "viewport") {
		t.Fatal("the mobile page has no original approval action")
	}
	request(http.MethodGet, taskURL+"/task", "", nil, http.StatusUnauthorized)
	noProject := grant("without-project", "publisher", false, true)
	noMachine := grant("without-machine", "publisher", true, false)
	reviewer := grant("reviewer", "reviewer", true, true)
	publisher := grant("publisher", "publisher", true, true)
	task := mobile(noMachine, http.MethodGet, "/task", "", http.StatusOK)
	if strings.Contains(string(task), fixture.head) || !strings.Contains(string(task), "https://github.com/owner/repo/issues/41") {
		t.Fatalf("task-only permission leaked verification or omitted the original Issue: %s", task)
	}
	mobile(noProject, http.MethodGet, "/task", "", http.StatusForbidden)
	mobile(noProject, http.MethodGet, "/evidence", "", http.StatusForbidden)
	mobile(noMachine, http.MethodGet, "/evidence", "", http.StatusForbidden)
	evidence := mobile(reviewer, http.MethodGet, "/evidence", "", http.StatusOK)
	if !strings.Contains(string(evidence), fixture.head) || !strings.Contains(string(evidence), `"canPublish":false`) {
		t.Fatalf("reader had no original SHA or gained publication: %s", evidence)
	}
	body := `{"id":"` + row.Id + `","digest":"` + strings.Repeat("0", 64) + `"}`
	if row.Digest != nil {
		body = fmt.Sprintf(`{"id":%q,"digest":%q}`, row.Id, *row.Digest)
	}
	for _, restricted := range []membership{noProject, noMachine, reviewer} {
		mobile(restricted, http.MethodPost, "/approve", body, http.StatusForbidden)
	}
	mobile(publisher, http.MethodGet, "/task", "", http.StatusOK)
	mobile(publisher, http.MethodGet, "/evidence", "", http.StatusOK)
	revoke(publisher)
	mobile(publisher, http.MethodGet, "/task", "", http.StatusForbidden)
	mobile(publisher, http.MethodGet, "/evidence", "", http.StatusForbidden)
	mobile(publisher, http.MethodPost, "/approve", body, http.StatusForbidden)
	if fixture.posts.Load() != 0 {
		t.Fatal("a restricted or revoked member published")
	}
	audit := readBoard(project.ID)
	if !slices.ContainsFunc(audit.Audit, func(event *testsupport.GitHubTeamAuditEvent) bool {
		return event.Action == "revoked" && event.GrantId == publisher.ID
	}) {
		t.Fatalf("revocation was not audited: %+v", audit.Audit)
	}

	r = reopenPluginRuntime(t, r, &cfg)
	waitConnected()
	linear("team-original", time.Now(), linearSecret, http.StatusOK)
	slack("CCODING", "", time.Now(), http.StatusOK)
	if after := readBoard(project.ID); !reflect.DeepEqual(initial.Sources, after.Sources) {
		t.Fatalf("redelivery after restart changed original sources: %+v", after.Sources)
	}
	mobile(publisher, http.MethodPost, "/approve", body, http.StatusForbidden)
	if row.Digest == nil {
		return
	}
	signer := grant("authorized-signoff", "publisher", true, true)
	receipt := mobile(signer, http.MethodPost, "/approve", body, http.StatusOK)
	if !strings.Contains(string(receipt), fixture.head) || !strings.Contains(string(receipt), "issuecomment-701") || fixture.posts.Load() != 1 {
		t.Fatalf("mobile authorization failed to use the original publication: %s", receipt)
	}
	if replay := mobile(signer, http.MethodPost, "/approve", body, http.StatusOK); string(replay) != string(receipt) || fixture.posts.Load() != 1 {
		t.Fatalf("a repeated approval replaced the original receipt: %s", replay)
	}
	durable := waitIssueDelivery(t, list, "41", "published")
	if durable.Receipt == nil || durable.Receipt.Head != fixture.head || durable.ApprovedAt == nil {
		t.Fatalf("the mobile approval did not settle the original Issue: %+v", durable)
	}
	after := readBoard(project.ID)
	if !slices.ContainsFunc(after.Audit, func(event *testsupport.GitHubTeamAuditEvent) bool {
		return event.Action == "publicationApproved" && event.GrantId == signer.ID
	}) {
		t.Fatalf("the original approval did not name its team grant: %+v", after.Audit)
	}
}
