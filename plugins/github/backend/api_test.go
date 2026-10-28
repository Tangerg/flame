package main

import (
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"
	"github.com/Tangerg/flame/qimu/portable"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func upstream(status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body))}
}

const issueJSON = `{"id":4200,"number":42,"title":"Inspected issue","body":"Original content","state":"open","labels":[],"assignees":[],"html_url":"https://github.com/owner/repo/issues/42","user":{"login":"author"},"updated_at":"2026-10-20T00:00:00Z"}`

func TestConditionalReadsRevalidateAuthorizationAndKeepCredentialScopesPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github.db")
	cache, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.db.Close() })
	token := "first-private-fixture-token"
	credential, err := newCredential(&token)
	if err != nil {
		t.Fatal(err)
	}
	status, calls := http.StatusOK, 0
	connector, err := newConnector(cache, roundTrip(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Host != "api.github.com" || request.URL.Path != "/repos/owner/repo/issues/42" || request.Header.Get("authorization") != "Bearer "+token || request.Header.Get("X-GitHub-Api-Version") != apiVersion {
			t.Error("request lost its resource or credential binding")
		}
		if status != http.StatusOK {
			return upstream(status, "", nil), nil
		}
		if calls > 1 && request.Header.Get("If-None-Match") != `"issue-1"` {
			t.Error("conditional read lost the original ETag")
		}
		if request.Header.Get("If-None-Match") != "" {
			return upstream(http.StatusNotModified, "", nil), nil
		}
		return upstream(http.StatusOK, issueJSON, http.Header{"Etag": {`"issue-1"`}}), nil
	}), credential)
	if err != nil {
		t.Fatal(err)
	}
	input := contractwire.IssueInput{Repository: "Owner/Repo", Number: 42}
	first, err := connector.getIssue(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := connector.getIssue(t.Context(), input)
	if err != nil || !reflect.DeepEqual(second, first) || calls != 2 {
		t.Fatalf("conditional query = %+v, count=%d, %v", second, calls, err)
	}
	status = http.StatusUnauthorized
	if _, err := connector.getIssue(t.Context(), input); err == nil {
		t.Fatal("revoked authorization returned cached private data")
	} else if refusal, ok := errors.AsType[*portable.Refusal](err); !ok || refusal.Code != "unauthorized" {
		t.Fatalf("authorization failure = %v", err)
	}
	otherToken := "second-private-fixture-token"
	other, err := newCredential(&otherToken)
	if err != nil {
		t.Fatal(err)
	}
	otherConnector, err := newConnector(cache, roundTrip(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("If-None-Match") != "" {
			t.Error("another credential borrowed an old conditional representation")
		}
		return upstream(http.StatusOK, issueJSON, nil), nil
	}), other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otherConnector.getIssue(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if err := cache.db.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), token) || strings.Contains(string(content), otherToken) {
		t.Fatal("connector database copied a mutable credential")
	}
	reopened, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.db.Close() })
	connector.store, status = reopened, http.StatusOK
	if original, err := connector.getIssue(t.Context(), input); err != nil || !reflect.DeepEqual(original, first) {
		t.Fatalf("conditional evidence did not survive reopen: %+v, %v", original, err)
	}
}

func TestGitHubReadsBoundResponsesAndPreserveExplicitFailures(t *testing.T) {
	cache, err := openStore(t.Context(), filepath.Join(t.TempDir(), "github.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.db.Close() })
	for _, test := range []struct {
		name          string
		status        int
		headers       http.Header
		body, refusal string
	}{
		{"unauthorized", 401, nil, "", "unauthorized"},
		{"forbidden", 403, nil, "", "forbidden"},
		{"rate limited", 403, http.Header{"X-Ratelimit-Remaining": {"0"}}, "", "rate_limited"},
		{"too many", 429, nil, "", "rate_limited"},
		{"not found", 404, nil, "", "not_found"},
		{"moved", 302, http.Header{"Location": {"https://other.example.test/steal"}}, "", "moved_resource"},
		{"malformed", 200, nil, "{", ""},
		{"missing original", 304, nil, "", ""},
		{"upstream failed", 500, nil, "", ""},
		{"oversized", 200, nil, strings.Repeat("x", maxResponseBytes+1), "response_too_large"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			connector, err := newConnector(cache, roundTrip(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.URL.Host != "api.github.com" {
					t.Fatal("redirect left the admitted API host")
				}
				return upstream(test.status, test.body, test.headers), nil
			}), credential{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = connector.getIssue(t.Context(), contractwire.IssueInput{Repository: "owner/repo", Number: 42})
			refusal, known := errors.AsType[*portable.Refusal](err)
			if err == nil || known != (test.refusal != "") || known && refusal.Code != test.refusal || calls != 1 {
				t.Fatalf("failure changed or retried: code=%v err=%v calls=%d", refusal, err, calls)
			}
		})
	}
	text := "Unicode source: 界\n"
	fileBody, err := json.Marshal(map[string]any{"type": "file", "path": "docs/read me.md", "sha": "original-sha", "html_url": "https://github.com/owner/repo/blob/main/docs/read%20me.md", "size": len(text), "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte(text))})
	if err != nil {
		t.Fatal(err)
	}
	connector, err := newConnector(cache, roundTrip(func(request *http.Request) (*http.Response, error) {
		if request.URL.EscapedPath() != "/repos/owner/repo/contents/docs/read%20me.md" || request.URL.Query().Get("ref") != "refs/tags/v1&exact" {
			t.Error("file path or ref escaped its parameter boundary")
		}
		return upstream(200, string(fileBody), nil), nil
	}), credential{})
	if err != nil {
		t.Fatal(err)
	}
	ref := "refs/tags/v1&exact"
	file, err := connector.readFile(t.Context(), contractwire.FileInput{Repository: "owner/repo", Path: "docs/read me.md", Ref: &ref})
	if err != nil || file.Text != text || file.SHA != "original-sha" {
		t.Fatalf("file = %+v, %v", file, err)
	}
}

func TestGitHubQueryCancellationJoinsItsOriginalRequest(t *testing.T) {
	cache, err := openStore(t.Context(), filepath.Join(t.TempDir(), "github.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.db.Close() })
	started := make(chan struct{})
	connector, err := newConnector(cache, roundTrip(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	}), credential{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := connector.getIssue(ctx, contractwire.IssueInput{Repository: "owner/repo", Number: 42})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation changed category: %v", err)
	}
}
