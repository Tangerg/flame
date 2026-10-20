package main

import (
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/plugins/github/backend/domain"
	"github.com/Tangerg/flame/plugins/github/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestTypedAndModelToolsShareGitHubReadsAndKnownRefusals(t *testing.T) {
	cache, err := openCache(t.Context(), filepath.Join(t.TempDir(), "github.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.db.Close() })
	denied := false
	connector, err := newConnector(cache, roundTrip(func(request *http.Request) (*http.Response, error) {
		if denied {
			return upstream(401, "", nil), nil
		}
		switch request.URL.Path {
		case "/repos/owner/repo/issues":
			if request.URL.Query().Get("per_page") != "2" || request.URL.Query().Get("state") != "all" {
				t.Error("list changed the inspected filters")
			}
			return upstream(200, "["+issueJSON+"]", http.Header{"Link": {`<https://api.github.com/repositories/123/issues?page=2&per_page=2>; rel="next", <https://api.github.com/repositories/123/issues?page=9&per_page=2>; rel="last"`}}), nil
		case "/repos/owner/repo/issues/42":
			return upstream(200, issueJSON, nil), nil
		case "/repos/owner/repo/contents/README.md":
			encoded, err := json.Marshal(map[string]any{"type": "file", "path": "README.md", "sha": "sha", "html_url": "https://github.com/owner/repo/blob/main/README.md", "size": 4, "encoding": "base64", "content": base64.StdEncoding.EncodeToString([]byte("text"))})
			if err != nil {
				return nil, err
			}
			return upstream(200, string(encoded), nil), nil
		default:
			t.Errorf("unexpected query path: %s", request.URL.Path)
			return upstream(404, "", nil), nil
		}
	}), domain.Credential{})
	if err != nil {
		t.Fatal(err)
	}
	server, err := newServer(connector)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "github-test"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	exported, err := pluginsdk.ResolveCapability(t.Context(), session, "github", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, tool string
		input        any
	}{
		{contracts.List.Name(), "list_issues", rpc.ListInput{Repository: "owner/repo", State: "all", Page: 1, Limit: 2}},
		{contracts.Issue.Name(), "get_issue", rpc.IssueInput{Repository: "owner/repo", Number: 42}},
		{contracts.File.Name(), "read_file", rpc.FileInput{Repository: "owner/repo", Path: "README.md"}},
	} {
		input, err := jsoninput.Encode(test.input)
		if err != nil {
			t.Fatal(err)
		}
		method, err := exported.Method(test.method)
		if err != nil {
			t.Fatal(err)
		}
		if method.Descriptor().Kind != capability.Query {
			t.Fatal("read operation acquired mutation authority")
		}
		typed, err := method.Invoke(t.Context(), "", input)
		if err != nil {
			t.Fatal(err)
		}
		result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: test.tool, Arguments: test.input})
		if err != nil || result.IsError {
			t.Fatalf("model Tool %s = %+v, %v", test.tool, result, err)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		model, err := jsoninput.Parse(encoded)
		if err != nil || model != typed {
			t.Fatalf("typed/Tool %s diverged: %s, %s, %v", test.tool, typed.String(), model.String(), err)
		}
		if test.tool == "list_issues" {
			var page rpc.IssuePage
			if err := json.Unmarshal(typed.JSON(), &page); err != nil || page.NextPage == nil || *page.NextPage != 2 {
				t.Fatalf("GitHub's canonical repository-ID pagination was lost: %+v, %v", page, err)
			}
		}
	}
	denied = true
	method, err := exported.Method(contracts.Issue.Name())
	if err != nil {
		t.Fatal(err)
	}
	input := rpc.IssueInput{Repository: "owner/repo", Number: 42}
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := method.Invoke(t.Context(), "", admitted); err == nil {
		t.Fatal("typed query hid rejected authorization")
	} else if refusal, ok := errors.AsType[*capability.Refusal](err); !ok || refusal.Code != "unauthorized" {
		t.Fatalf("typed refusal = %v", err)
	}
	result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "get_issue", Arguments: input})
	if err != nil || !result.IsError || len(result.Content) != 1 || !strings.Contains(result.Content[0].(*sdk.TextContent).Text, "credential was rejected") {
		t.Fatalf("known Tool refusal became a protocol error or success: %+v, %v", result, err)
	}
}
