package main

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	rpc "github.com/Tangerg/flame/plugins/github/internal/contract"
	contractwire "github.com/Tangerg/flame/plugins/github/internal/wire"

	framecall "github.com/Tangerg/frame/call"

	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

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
	if err := pluginmcp.RegisterClient(client); err != nil {
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

func githubBinding(t *testing.T, session *sdk.ClientSession, contracts rpc.Contracts, method string) *pluginmcp.Binding {
	t.Helper()
	bound, err := pluginmcp.ResolveCapability(t.Context(), session, "github", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := bound.Method(method)
	if err != nil {
		t.Fatal(err)
	}
	return operation
}

func invokeGitHub(t *testing.T, session *sdk.ClientSession, contracts rpc.Contracts, method, id string, input framevalue.JSON) (framevalue.JSON, error) {
	t.Helper()
	ctx, err := pluginmcp.WithInvocation(t.Context(), framecall.Invocation{ID: id, Origin: extension.Human, Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: "github-owner"}})
	if err != nil {
		t.Fatal(err)
	}
	return githubBinding(t, session, contracts, method).Invoke(ctx, pluginmcp.CommandID(id), input)
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
				if err := c.store.db.RunInTx(t.Context(), func(ctx context.Context) error {
					_, err := c.store.db.Executor(ctx).ExecContext(ctx, `CREATE TRIGGER refuse_settlement BEFORE UPDATE OF reply ON commands WHEN NEW.reply IS NOT NULL BEGIN SELECT RAISE(FAIL,'fixture commit failed'); END`)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			input, _ := framevalue.EncodeJSON(contractwire.CreateIssueInput{Repository: "owner/repo", Title: "Original"})
			_, err := invokeGitHub(t, session, contracts, contracts.CreateIssue.Name(), "original", input)
			if err == nil {
				t.Fatal("uncertain write invented success")
			}
			if _, known := errors.AsType[*framecall.Refusal](err); known {
				t.Fatalf("uncertain write became a definite refusal: %v", err)
			}
			if calls != 1 {
				t.Fatalf("uncertain request was retried: %d", calls)
			}
			if failure == "commit" {
				if err := c.store.db.RunInTx(t.Context(), func(ctx context.Context) error {
					_, err := c.store.db.Executor(ctx).ExecContext(ctx, `DROP TRIGGER refuse_settlement`)
					return err
				}); err != nil {
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
			changed, _ := framevalue.EncodeJSON(contractwire.CreateIssueInput{Repository: "owner/repo", Title: "Different"})
			_, err = invokeGitHub(t, session, contracts, contracts.CreateIssue.Name(), "original", changed)
			refusal, known := errors.AsType[*framecall.Refusal](err)
			if !known || refusal.Code != "command_conflict" || calls != 1 {
				t.Fatalf("unknown identity was rebound: calls=%d %v", calls, err)
			}
		})
	}
}
