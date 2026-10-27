package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const inspectedSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const mergedSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func connectGitHubWrites(t *testing.T, path string, transport http.RoundTripper, token *string) (*connector, *sdk.ClientSession, func()) {
	t.Helper()
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := newCredential(token)
	if err != nil {
		t.Fatal(err)
	}
	c, err := newConnector(store, transport, credential)
	if err != nil {
		t.Fatal(err)
	}
	server, err := newServer(c)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "github-write-test"}, nil)
	if err := qimumcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	close := func() {
		once.Do(func() {
			if err := errors.Join(session.Close(), backend.Close(), store.db.Close()); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(close)
	return c, session, close
}

func githubBinding(t *testing.T, session *sdk.ClientSession, contracts rpc.Contracts, method string) *qimumcp.Binding {
	t.Helper()
	bound, err := qimumcp.ResolveCapability(t.Context(), session, "github", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := bound.Method(method)
	if err != nil {
		t.Fatal(err)
	}
	return operation
}

func invokeGitHub(t *testing.T, session *sdk.ClientSession, contracts rpc.Contracts, method, id string, input jsonvalue.Value) (jsonvalue.Value, error) {
	t.Helper()
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: id, Origin: extension.Human, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "github-owner"}})
	if err != nil {
		t.Fatal(err)
	}
	return githubBinding(t, session, contracts, method).Invoke(ctx, id, input)
}

func TestGitHubTypedAndModelWritesShareAdmissionOutcomesAndRecovery(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	issue := rpc.IssueInput{Repository: "owner/repo", Number: 42}
	comment := rpc.CommentInput{Repository: "owner/repo", CommentID: 11}
	inlineID := rpc.CommentInput{Repository: "owner/repo", CommentID: 33}
	location := rpc.InlineComment{Path: "new.go", Body: "Review note", Line: 7, Side: rpc.Right, StartLine: new(int64(5)), StartSide: new(rpc.Right)}
	submitted := strings.ReplaceAll(strings.ReplaceAll(reviewJSON, `"state":"PENDING","commit_id":null,"submitted_at":null`, `"state":"APPROVED","commit_id":"`+inspectedSHA+`","submitted_at":"2026-10-20T00:00:00Z"`), `"body":""`, `"body":"Looks good"`)
	inline := strings.ReplaceAll(reviewCommentJSON, `"commit_id":"head-sha"`, `"commit_id":"`+inspectedSHA+`"`)
	for _, test := range []struct {
		name, method, verb, path, body string
		status                         int
		input                          any
		payload                        string
	}{
		{"create_issue", contracts.CreateIssue.Name(), "POST", "/repos/owner/repo/issues", issueJSON, 201, rpc.CreateIssueInput{Repository: "owner/repo", Title: "New issue", Body: new("Text"), Labels: []string{"bug"}, Assignees: []string{"octocat"}}, `{"title":"New issue","body":"Text","labels":["bug"],"assignees":["octocat"]}`},
		{"update_issue", contracts.UpdateIssue.Name(), "PATCH", "/repos/owner/repo/issues/42", issueJSON, 200, rpc.UpdateIssueInput{IssueInput: issue, Body: new(""), State: new(rpc.Closed), StateReason: new(rpc.NotPlanned), Labels: new([]string{}), Assignees: new([]string{})}, `{"body":"","state":"closed","state_reason":"not_planned","labels":[],"assignees":[]}`},
		{"create_issue_comment", contracts.CreateComment.Name(), "POST", "/repos/owner/repo/issues/42/comments", commentJSON, 201, rpc.CreateCommentInput{IssueInput: issue, Body: "Discussion"}, `{"body":"Discussion"}`},
		{"update_issue_comment", contracts.UpdateComment.Name(), "PATCH", "/repos/owner/repo/issues/comments/11", commentJSON, 200, rpc.UpdateCommentInput{CommentInput: comment, Body: "Edited"}, `{"body":"Edited"}`},
		{"delete_issue_comment", contracts.DeleteComment.Name(), "DELETE", "/repos/owner/repo/issues/comments/11", "", 204, comment, ""},
		{"create_pull_request", contracts.CreatePull.Name(), "POST", "/repos/owner/repo/pulls", strings.Replace(pullJSON, `"draft":true`, `"draft":true,"maintainer_can_modify":false`, 1), 201, rpc.CreatePullInput{Repository: "owner/repo", Title: "New PR", Head: "fork:topic", Base: "main", HeadRepository: new("fork-repo"), Draft: new(false), MaintainerCanModify: new(false)}, `{"title":"New PR","head":"fork:topic","base":"main","head_repo":"fork-repo","draft":false,"maintainer_can_modify":false}`},
		{"submit_pull_request_review", contracts.SubmitReview.Name(), "POST", "/repos/owner/repo/pulls/42/reviews", submitted, 200, rpc.SubmitReviewInput{IssueInput: issue, CommitID: inspectedSHA, Event: rpc.Approve, Body: new("Looks good"), Comments: []rpc.InlineComment{location}}, `{"commit_id":"` + inspectedSHA + `","event":"APPROVE","body":"Looks good","comments":[{"path":"new.go","body":"Review note","line":7,"side":"RIGHT","start_line":5,"start_side":"RIGHT"}]}`},
		{"merge_pull_request", contracts.MergePull.Name(), "PUT", "/repos/owner/repo/pulls/42/merge", `{"sha":"` + mergedSHA + `","merged":true,"message":"Merged"}`, 200, rpc.MergePullInput{IssueInput: issue, HeadSHA: inspectedSHA, MergeMethod: rpc.Squash, CommitTitle: new("Squashed"), CommitMessage: new("")}, `{"sha":"` + inspectedSHA + `","merge_method":"squash","commit_title":"Squashed","commit_message":""}`},
		{"create_pull_request_comment", contracts.CreateReviewComment.Name(), "POST", "/repos/owner/repo/pulls/42/comments", inline, 201, rpc.CreateReviewCommentInput{IssueInput: issue, CommitID: inspectedSHA, InlineComment: location}, `{"commit_id":"` + inspectedSHA + `","path":"new.go","body":"Review note","line":7,"side":"RIGHT","start_line":5,"start_side":"RIGHT"}`},
		{"reply_pull_request_comment", contracts.ReplyReviewComment.Name(), "POST", "/repos/owner/repo/pulls/42/comments/32/replies", inline, 201, rpc.ReplyReviewCommentInput{IssueInput: issue, CommentID: 32, Body: "Thread reply"}, `{"body":"Thread reply"}`},
		{"update_pull_request_comment", contracts.UpdateReviewComment.Name(), "PATCH", "/repos/owner/repo/pulls/comments/33", inline, 200, rpc.UpdateCommentInput{CommentInput: inlineID, Body: "Edited review"}, `{"body":"Edited review"}`},
		{"delete_pull_request_comment", contracts.DeleteReviewComment.Name(), "DELETE", "/repos/owner/repo/pulls/comments/33", "", 204, inlineID, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "github.db")
			token, calls, commandID := "private-write-fixture-token", 0, "human"
			var c *connector
			transport := roundTrip(func(request *http.Request) (*http.Response, error) {
				calls++
				var pending int
				if err := c.store.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM commands WHERE id=? AND reply IS NULL`, commandID).Scan(&pending); err != nil || pending != 1 {
					t.Errorf("HTTP preceded durable admission: pending=%d %v", pending, err)
				}
				if request.Method != test.verb || request.URL.String() != apiOrigin+test.path || request.Header.Get("Authorization") != "Bearer "+token || request.Header.Get("X-GitHub-Api-Version") != apiVersion || request.GetBody != nil || request.Header.Get("Idempotency-Key") != "" {
					t.Errorf("write lost its exact target or authority: %s %s", request.Method, request.URL)
				}
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatal(err)
				}
				if test.payload == "" {
					if len(body) != 0 {
						t.Errorf("delete carried a different payload: %s", body)
					}
				} else {
					actual, err := jsonvalue.Parse(body)
					expected, _ := jsonvalue.Parse([]byte(test.payload))
					if err != nil || actual != expected {
						t.Errorf("wire write differs: got %s want %s %v", body, test.payload, err)
					}
				}
				return upstream(test.status, test.body, nil), nil
			})
			var session *sdk.ClientSession
			var close func()
			c, session, close = connectGitHubWrites(t, path, transport, &token)
			admitted, err := jsonvalue.Encode(test.input)
			if err != nil {
				t.Fatal(err)
			}
			value, err := invokeGitHub(t, session, contracts, test.method, commandID, admitted)
			if err != nil {
				t.Fatal(err)
			}
			if replay, err := invokeGitHub(t, session, contracts, test.method, commandID, admitted); err != nil || replay != value || calls != 1 {
				t.Fatalf("human replay redispatched: %s calls=%d %v", replay.String(), calls, err)
			}
			var meta sdk.Meta
			if err := meta.Set(qimumcp.InvocationMetadataKey, portable.Invocation{ID: "model", Origin: extension.Model, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "github-owner"}}); err != nil {
				t.Fatal(err)
			}
			commandID = "tool:model"
			result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: test.name, Arguments: test.input, Meta: meta})
			if err != nil || result.IsError {
				t.Fatalf("model write failed: %+v %v", result, err)
			}
			encoded, _ := json.Marshal(result.StructuredContent)
			projected, err := jsonvalue.Parse(encoded)
			if err != nil || projected != value || calls != 2 {
				t.Fatalf("typed/model outcome diverged: %s %s calls=%d %v", value.String(), encoded, calls, err)
			}
			original, err := pluginsdk.ReadToolReceipt(t.Context(), session, "model", test.name, admitted)
			if err != nil || original == nil || calls != 2 {
				t.Fatalf("model receipt dispatched HTTP: %+v %v", original, err)
			}
			close()
			c, session, _ = connectGitHubWrites(t, path, transport, &token)
			if replay, err := invokeGitHub(t, session, contracts, test.method, "human", admitted); err != nil || replay != value || calls != 2 {
				t.Fatalf("restart repeated human write: %s calls=%d %v", replay.String(), calls, err)
			}
			output, err := pluginsdk.ReadToolReceipt(t.Context(), session, "model", test.name, admitted)
			if err != nil || !reflect.DeepEqual(output, original) || calls != 2 {
				t.Fatalf("restart changed model receipt: %+v %v", output, err)
			}
		})
	}
}

func TestUnknownGitHubWriteNeverRedispatchesAfterRestart(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, failure := range []string{"transport", "server", "malformed", "commit", "redirect", "deadline"} {
		t.Run(failure, func(t *testing.T) {
			path, token := filepath.Join(t.TempDir(), "github.db"), "private-write-fixture-token"
			calls := 0
			transport := roundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				switch failure {
				case "transport":
					return nil, errors.New("fixture: response lost after upstream accepted")
				case "server":
					return upstream(502, "upstream processing uncertain", nil), nil
				case "malformed":
					return upstream(201, `{"number":42}`, nil), nil
				case "redirect":
					return upstream(302, "", http.Header{"Location": {"https://other.example/credential"}}), nil
				case "deadline":
					return nil, context.DeadlineExceeded
				default:
					return upstream(201, issueJSON, nil), nil
				}
			})
			c, session, close := connectGitHubWrites(t, path, transport, &token)
			if failure == "commit" {
				if _, err := c.store.db.ExecContext(t.Context(), `CREATE TRIGGER refuse_settlement BEFORE UPDATE OF reply ON commands WHEN NEW.reply IS NOT NULL BEGIN SELECT RAISE(FAIL,'fixture commit failed'); END`); err != nil {
					t.Fatal(err)
				}
			}
			input, _ := jsonvalue.Encode(rpc.CreateIssueInput{Repository: "owner/repo", Title: "Original"})
			_, err := invokeGitHub(t, session, contracts, contracts.CreateIssue.Name(), "original", input)
			if err == nil {
				t.Fatal("uncertain write invented success")
			}
			if _, known := errors.AsType[*portable.Refusal](err); known {
				t.Fatalf("uncertain write became a definite refusal: %v", err)
			}
			if calls != 1 {
				t.Fatalf("uncertain request was retried: %d", calls)
			}
			if failure == "commit" {
				if _, err := c.store.db.ExecContext(t.Context(), `DROP TRIGGER refuse_settlement`); err != nil {
					t.Fatal(err)
				}
			}
			close()
			_, session, _ = connectGitHubWrites(t, path, roundTrip(func(*http.Request) (*http.Response, error) { calls++; return upstream(201, issueJSON, nil), nil }), &token)
			if _, err := invokeGitHub(t, session, contracts, contracts.CreateIssue.Name(), "original", input); err == nil || calls != 1 {
				t.Fatalf("restart redispatched unknown command: calls=%d %v", calls, err)
			}
			_, found, err := githubBinding(t, session, contracts, contracts.CreateIssue.Name()).ReadReceipt(t.Context(), "original", input)
			if err != nil || found || calls != 1 {
				t.Fatalf("receipt lookup invented an outcome: found=%v calls=%d %v", found, calls, err)
			}
			changed, _ := jsonvalue.Encode(rpc.CreateIssueInput{Repository: "owner/repo", Title: "Different"})
			_, err = invokeGitHub(t, session, contracts, contracts.CreateIssue.Name(), "original", changed)
			refusal, known := errors.AsType[*portable.Refusal](err)
			if !known || refusal.Code != "command_conflict" || calls != 1 {
				t.Fatalf("unknown identity was rebound: calls=%d %v", calls, err)
			}
		})
	}
}

func TestGitHubKnownWriteRefusalsAndOriginalModelReceiptSurviveLaterSuccess(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{400, 401, 403, 404, 405, 409, 422, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			path, calls, token := filepath.Join(t.TempDir(), "github.db"), 0, "private-write-fixture-token"
			_, session, close := connectGitHubWrites(t, path, roundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				return upstream(status, "do not echo provider body or credential", nil), nil
			}), &token)
			input := rpc.MergePullInput{IssueInput: rpc.IssueInput{Repository: "owner/repo", Number: 42}, HeadSHA: inspectedSHA, MergeMethod: rpc.Merge}
			admitted, _ := jsonvalue.Encode(input)
			var meta sdk.Meta
			if err := meta.Set(qimumcp.InvocationMetadataKey, portable.Invocation{ID: "refused", Origin: extension.Model, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "github-owner"}}); err != nil {
				t.Fatal(err)
			}
			result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "merge_pull_request", Arguments: input, Meta: meta})
			if err != nil || !result.IsError {
				t.Fatalf("known refusal did not reach model feedback: %+v %v", result, err)
			}
			original, err := pluginsdk.ReadToolReceipt(t.Context(), session, "refused", "merge_pull_request", admitted)
			if original == nil || err == nil || calls != 1 {
				t.Fatalf("known refusal was not retained: %+v %v", original, err)
			}
			close()
			_, session, _ = connectGitHubWrites(t, path, roundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				return upstream(200, `{"sha":"`+mergedSHA+`","merged":true,"message":"Merged"}`, nil), nil
			}), &token)
			recovered, err := pluginsdk.ReadToolReceipt(t.Context(), session, "refused", "merge_pull_request", admitted)
			if err == nil || !reflect.DeepEqual(recovered, original) || calls != 1 {
				t.Fatalf("restart reevaluated original refusal: %+v calls=%d %v", recovered, calls, err)
			}
			if _, err := invokeGitHub(t, session, contracts, contracts.MergePull.Name(), "new-intent", admitted); err != nil || calls != 2 {
				t.Fatalf("explicit new command could not observe later permission: %v", err)
			}
		})
	}
}

func TestGitHubWriteInvalidatesReadProjectionsBeforeExternalIO(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	var c *connector
	var readTags []string
	token := "private-write-fixture-token"
	c, session, _ := connectGitHubWrites(t, filepath.Join(t.TempDir(), "github.db"), roundTrip(func(request *http.Request) (*http.Response, error) {
		if request.Method == http.MethodGet {
			readTags = append(readTags, request.Header.Get("If-None-Match"))
			return upstream(200, issueJSON, http.Header{"Etag": {`"original"`}}), nil
		}
		var cached int
		if err := c.store.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM responses`).Scan(&cached); err != nil || cached != 0 {
			t.Errorf("write left pre-write projections available: %d %v", cached, err)
		}
		return nil, errors.New("fixture: response lost")
	}), &token)
	input := rpc.IssueInput{Repository: "owner/repo", Number: 42}
	if _, err := c.getIssue(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	admitted, _ := jsonvalue.Encode(rpc.CreateCommentInput{IssueInput: input, Body: "Comment"})
	if _, err := invokeGitHub(t, session, contracts, contracts.CreateComment.Name(), "unknown-write", admitted); err == nil {
		t.Fatal("unknown comment invented success")
	}
	if _, err := c.getIssue(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(readTags, []string{"", ""}) {
		t.Fatalf("post-write query borrowed an old conditional response: %+v", readTags)
	}
	if receipt, err := pluginstorage.ReadReceipt(t.Context(), c.store.db, qimumcp.ReceiptRequest{ID: "unknown-write", Method: contracts.CreateComment.Name(), InputDigest: admitted.Digest()}); err != nil || receipt.Found {
		t.Fatalf("unknown write acquired a receipt: %+v %v", receipt, err)
	}
}
