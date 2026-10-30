package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	rpc "github.com/Tangerg/flame/plugins/web/internal/contract"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/core/tool"
	"github.com/Tangerg/scope/tools/httpreq"
)

func httpFixture(t *testing.T, config httpreq.ClientConfig) (*requests, string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "web.db")
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.db.Close() })
	requests, err := newRequests(store.db, config)
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	return requests, contracts.Request.Name(), path
}

func httpInput(t *testing.T, request httpreq.Request) jsonvalue.Value {
	t.Helper()
	input, err := jsonvalue.Encode(request)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestHTTPOriginalResultSurvivesRestartWithoutAnotherRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, "original response")
	}))
	t.Cleanup(server.Close)
	requests, method, path := httpFixture(t, httpreq.ClientConfig{AllowedHosts: []string{"127.0.0.1"}})
	input := httpInput(t, httpreq.Request{URL: server.URL})
	first, err := requests.execute(t.Context(), method, "original", input)
	if err != nil || first.Status != http.StatusOK {
		t.Fatalf("request: %+v %v", first, err)
	}
	if err := requests.db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.db.Close() })
	// A new owner with no network grant can still read the original result.
	restarted, err := newRequests(store.db, httpreq.ClientConfig{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := restarted.execute(t.Context(), method, "original", input)
	if err != nil || second.Duration != first.Duration || calls.Load() != 1 {
		t.Fatalf("replay: %+v %v, calls=%d", second, err, calls.Load())
	}
	if second.Body.Encoding != "utf8" || second.Body.Data != "original response" {
		t.Fatalf("original body: %+v", second.Body)
	}
	changed := httpInput(t, httpreq.Request{URL: server.URL + "/changed"})
	if _, err := restarted.execute(t.Context(), method, "original", changed); !errors.Is(err, portable.ErrInvocationConflict) {
		t.Fatalf("changed request reused identity: %v", err)
	}
	receipt, err := restarted.receipt(t.Context(), qimumcp.ReceiptRequest{ID: "original", Method: method, InputDigest: input.Digest()})
	if err != nil || !receipt.Found || receipt.IsError {
		t.Fatalf("original tool receipt: %+v %v", receipt, err)
	}
}

func TestHTTPPolicyRefusalsNeverReachTransport(t *testing.T) {
	for _, test := range []struct {
		name  string
		input httpreq.Request
		hosts []string
	}{
		{name: "no grant", input: httpreq.Request{URL: "https://allowed.test"}},
		{name: "host", hosts: []string{"allowed.test"}, input: httpreq.Request{URL: "https://denied.test"}},
		{name: "method", hosts: []string{"allowed.test"}, input: httpreq.Request{URL: "https://allowed.test", Method: httpreq.MethodPOST}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: httpTransport(func(*http.Request) (*http.Response, error) {
				t.Fatal("policy refusal reached network transport")
				return nil, errors.New("unexpected dispatch")
			})}
			requests, method, _ := httpFixture(t, httpreq.ClientConfig{AllowedHosts: test.hosts, HTTPClient: client})
			_, err := requests.execute(t.Context(), method, "refused", httpInput(t, test.input))
			if _, known := errors.AsType[*portable.Refusal](err); !known {
				t.Fatalf("policy failure is unconfirmed: %v", err)
			}
			if _, known := tool.ClassifyFailure(requestToolFailure(err)); !known {
				t.Fatalf("model policy failure is unconfirmed: %v", err)
			}
		})
	}
}

type httpTransport func(*http.Request) (*http.Response, error)

func (f httpTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestHTTPUnconfirmedDispatchCannotReplayOrBecomeAReceipt(t *testing.T) {
	for _, failure := range []error{errors.New("response lost"), context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			var calls int
			client := &http.Client{Transport: httpTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, failure
			})}
			requests, method, _ := httpFixture(t, httpreq.ClientConfig{AllowedHosts: []string{"allowed.test"}, HTTPClient: client})
			input := httpInput(t, httpreq.Request{URL: "https://allowed.test"})
			_, err := requests.execute(t.Context(), method, "unconfirmed", input)
			if !errors.Is(err, failure) {
				t.Fatalf("original failure lost: %v", err)
			}
			if _, known := tool.ClassifyFailure(requestToolFailure(err)); known {
				t.Fatal("unconfirmed request became a definite failure")
			}
			if _, err := requests.execute(t.Context(), method, "unconfirmed", input); !errors.Is(err, pluginstorage.ErrCommandUnconfirmed) || calls != 1 {
				t.Fatalf("unconfirmed replay: %v, calls=%d", err, calls)
			}
			receipt, err := requests.receipt(t.Context(), qimumcp.ReceiptRequest{ID: "unconfirmed", Method: method, InputDigest: input.Digest()})
			if err != nil || receipt.Found {
				t.Fatalf("invented original outcome: %+v %v", receipt, err)
			}
		})
	}
}

func TestHTTPResponseBoundAndRedirectGrant(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path == "/redirect" {
			http.Redirect(w, request, "https://denied.test/resource", http.StatusTemporaryRedirect)
			return
		}
		_, _ = io.WriteString(w, strings.Repeat("x", int(httpreq.DefaultMaxResponseBytes)+1))
	}))
	t.Cleanup(server.Close)
	requests, method, _ := httpFixture(t, httpreq.ClientConfig{AllowedHosts: []string{"127.0.0.1"}})
	result, err := requests.execute(t.Context(), method, "bounded", httpInput(t, httpreq.Request{URL: server.URL}))
	if err != nil || !result.Truncated || len(result.Body.Data) != int(httpreq.DefaultMaxResponseBytes) {
		t.Fatalf("response bound: bytes=%d truncated=%v error=%v", len(result.Body.Data), result.Truncated, err)
	}
	input := httpInput(t, httpreq.Request{URL: server.URL + "/redirect"})
	_, err = requests.execute(t.Context(), method, "redirect", input)
	if !errors.Is(err, httpreq.ErrHostNotAllowed) || calls.Load() != 2 {
		t.Fatalf("redirect grant: calls=%d error=%v", calls.Load(), err)
	}
	if _, known := tool.ClassifyFailure(requestToolFailure(err)); known {
		t.Fatal("post-dispatch redirect refusal proved the original request had no effect")
	}
}
