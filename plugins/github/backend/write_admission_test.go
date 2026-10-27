package main

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
)

func TestGitHubWriteLocalRefusalsSettleBeforeAnyHTTP(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	issue := contractwire.IssueInput{Repository: "owner/repo", Number: 42}
	for _, test := range []struct {
		name, method, code string
		input              any
	}{
		{"blank_title", contracts.CreateIssue.Name(), "invalid_input", contractwire.CreateIssueInput{Repository: "owner/repo", Title: "  "}},
		{"path_escape", contracts.CreateIssue.Name(), "invalid_resource", contractwire.CreateIssueInput{Repository: "../private", Title: "Title"}},
		{"no_patch", contracts.UpdateIssue.Name(), "invalid_input", contractwire.UpdateIssueInput{IssueInput: issue}},
		{"ignored_reason", contracts.UpdateIssue.Name(), "invalid_input", contractwire.UpdateIssueInput{IssueInput: issue, StateReason: new(contractwire.Completed)}},
		{"bad_reason", contracts.UpdateIssue.Name(), "invalid_input", contractwire.UpdateIssueInput{IssueInput: issue, State: new(contractwire.Open), StateReason: new(contractwire.NotPlanned)}},
		{"duplicate_without_id", contracts.UpdateIssue.Name(), "invalid_input", contractwire.UpdateIssueInput{IssueInput: issue, State: new(contractwire.Closed), StateReason: new(contractwire.Duplicate)}},
		{"duplicate_names", contracts.CreateIssue.Name(), "invalid_input", contractwire.CreateIssueInput{Repository: "owner/repo", Title: "Title", Labels: []string{"bug", "bug"}}},
		{"blank_comment", contracts.CreateComment.Name(), "invalid_input", contractwire.CreateCommentInput{IssueInput: issue, Body: "  "}},
		{"blank_branch", contracts.CreatePull.Name(), "invalid_input", contractwire.CreatePullInput{Repository: "owner/repo", Title: "Title", Head: " ", Base: "main"}},
		{"review_missing_body", contracts.SubmitReview.Name(), "invalid_input", contractwire.SubmitReviewInput{IssueInput: issue, CommitID: inspectedSHA, Event: contractwire.RequestChanges}},
		{"incomplete_range", contracts.CreateReviewComment.Name(), "invalid_input", contractwire.CreateReviewCommentInput{IssueInput: issue, CommitID: inspectedSHA, InlineComment: contractwire.InlineComment{Path: "file.go", Body: "Comment", Line: 7, Side: contractwire.Right, StartLine: new(int64(5))}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls, token := 0, "private-write-fixture-token"
			_, session, _ := connectGitHubWrites(t, filepath.Join(t.TempDir(), "github.db"), roundTrip(func(*http.Request) (*http.Response, error) { calls++; return upstream(201, issueJSON, nil), nil }), &token)
			input, _ := jsonvalue.Encode(test.input)
			for range 2 {
				_, err := invokeGitHub(t, session, contracts, test.method, "original", input)
				refusal, known := errors.AsType[*portable.Refusal](err)
				if !known || refusal.Code != test.code || calls != 0 {
					t.Fatalf("local refusal lost settlement or dispatched HTTP: calls=%d %v", calls, err)
				}
			}
			_, found, err := githubBinding(t, session, contracts, test.method).ReadReceipt(t.Context(), "original", input)
			if _, known := errors.AsType[*portable.Refusal](err); !found || !known || calls != 0 {
				t.Fatalf("local refusal has no original receipt: found=%v calls=%d %v", found, calls, err)
			}
		})
	}
}

func TestGitHubMissingWriteCredentialIsAnOriginalRefusal(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	path, calls := filepath.Join(t.TempDir(), "github.db"), 0
	transport := roundTrip(func(*http.Request) (*http.Response, error) { calls++; return upstream(201, issueJSON, nil), nil })
	_, session, close := connectGitHubWrites(t, path, transport, nil)
	input, _ := jsonvalue.Encode(contractwire.CreateIssueInput{Repository: "owner/repo", Title: "Title"})
	_, err = invokeGitHub(t, session, contracts, contracts.CreateIssue.Name(), "original", input)
	if refusal, known := errors.AsType[*portable.Refusal](err); !known || refusal.Code != "credential_required" || calls != 0 {
		t.Fatalf("anonymous write contacted upstream: calls=%d %v", calls, err)
	}
	close()
	token := "private-write-fixture-token"
	_, session, _ = connectGitHubWrites(t, path, transport, &token)
	_, err = invokeGitHub(t, session, contracts, contracts.CreateIssue.Name(), "original", input)
	if refusal, known := errors.AsType[*portable.Refusal](err); !known || refusal.Code != "credential_required" || calls != 0 {
		t.Fatalf("new credential changed the original refusal: calls=%d %v", calls, err)
	}
}

func TestGitHubWriteSchemasOwnExactCommitAndTextAdmission(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	merge, err := portable.AdmitMethod(contracts.MergePull.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	for _, sha := range []string{"", "main", strings.Repeat("g", 40), inspectedSHA} {
		input, _ := jsonvalue.Encode(contractwire.MergePullInput{IssueInput: contractwire.IssueInput{Repository: "owner/repo", Number: 42}, HeadSHA: sha, MergeMethod: contractwire.Merge})
		if err := merge.ValidateInput(input); (err == nil) != (sha == inspectedSHA) {
			t.Fatalf("merge head admission %q: %v", sha, err)
		}
	}
	comment, err := portable.AdmitMethod(contracts.CreateComment.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{contractwire.MaxBodyRunes, contractwire.MaxBodyRunes + 1} {
		input, _ := jsonvalue.Encode(contractwire.CreateCommentInput{IssueInput: contractwire.IssueInput{Repository: "owner/repo", Number: 42}, Body: strings.Repeat("文", count)})
		if err := comment.ValidateInput(input); (err == nil) != (count == contractwire.MaxBodyRunes) {
			t.Fatalf("Unicode body admission %d: %v", count, err)
		}
	}
	for _, reply := range []string{`{"merged":false,"sha":"sha","message":"not merged"}`, `null`, `{}`} {
		if err := merge.ValidateOutput([]byte(reply)); err == nil {
			t.Fatalf("incomplete merge acknowledgement was admitted: %s", reply)
		}
	}
}

func TestGitHubCorruptToolReceiptCannotBecomeAnEmptySuccessfulWrite(t *testing.T) {
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	token, calls := "private-write-fixture-token", 0
	c, session, _ := connectGitHubWrites(t, filepath.Join(t.TempDir(), "github.db"), roundTrip(func(*http.Request) (*http.Response, error) { calls++; return upstream(201, issueJSON, nil), nil }), &token)
	input, _ := jsonvalue.Encode(contractwire.CreateIssueInput{Repository: "owner/repo", Title: "Title"})
	for _, reply := range []string{`{"value":null}`, `{"value":{}}`} {
		if _, err := c.store.db.ExecContext(t.Context(), `INSERT INTO commands(id,method,business_input_json,reply) VALUES('tool:corrupt',?,?,?) ON CONFLICT(id) DO UPDATE SET reply=excluded.reply`, contracts.CreateIssue.Name(), input.String(), reply); err != nil {
			t.Fatal(err)
		}
		output, err := pluginsdk.ReadToolReceipt(t.Context(), session, "corrupt", "create_issue", input)
		if output != nil || err == nil || calls != 0 {
			t.Fatalf("corrupt receipt became successful or dispatched: %+v calls=%d %v", output, calls, err)
		}
	}
}

func TestGitHubCacheOnlyDataIsRefusedWithoutAddingCommandTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github.db")
	old := []string{`CREATE TABLE IF NOT EXISTS responses (key TEXT PRIMARY KEY, etag TEXT NOT NULL, links TEXT NOT NULL, body BLOB NOT NULL, checked_at INTEGER NOT NULL)`}
	db, err := sqlite.OpenSchema(t.Context(), path, old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO responses VALUES('original','etag','',X'7B7D',1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if replacement, err := openStore(t.Context(), path); err == nil {
		_ = replacement.db.Close()
		t.Fatal("old cache-only data was converted")
	}
	db, err = sqlite.OpenSchema(context.Background(), path, old)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM responses WHERE key='original'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("refused opening changed original data: %d %v", count, err)
	}
}
