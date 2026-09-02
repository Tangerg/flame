package config

import (
	"reflect"
	"strings"
	"testing"
)

// TestParseMCPServers covers the env-var parser across both HTTP
// and stdio transport syntaxes plus error cases.
func TestParseMCPServers(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    []MCPServer
		wantErr bool
	}{
		{
			name: "empty input → nil",
			in:   "",
			want: nil,
		},
		{
			name: "single http entry",
			in:   "github=https://mcp.github.com/",
			want: []MCPServer{{
				Name:      "github",
				Transport: MCPTransportStreamableHTTP,
				Endpoint:  "https://mcp.github.com/",
			}},
		},
		{
			name: "single stdio entry",
			in:   "fs=stdio:npx -y @modelcontextprotocol/server-filesystem /workspace",
			want: []MCPServer{{
				Name:      "fs",
				Transport: MCPTransportStdio,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-filesystem", "/workspace"},
			}},
		},
		{
			name: "stdio with single-word command",
			in:   "time=stdio:mcp-server-time",
			want: []MCPServer{{
				Name:      "time",
				Transport: MCPTransportStdio,
				Command:   "mcp-server-time",
				Args:      []string{},
			}},
		},
		{
			name: "mixed http + stdio with whitespace",
			in:   " github = https://mcp.github.com/ , fs = stdio:npx mcp-server-fs ",
			want: []MCPServer{
				{Name: "github", Transport: MCPTransportStreamableHTTP, Endpoint: "https://mcp.github.com/"},
				{Name: "fs", Transport: MCPTransportStdio, Command: "npx", Args: []string{"mcp-server-fs"}},
			},
		},
		{
			name:    "missing equals",
			in:      "github",
			wantErr: true,
		},
		{
			name:    "trailing equals",
			in:      "github=",
			wantErr: true,
		},
		{
			name:    "empty name",
			in:      "=https://mcp/",
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			in:      "github=ftp://mcp/",
			wantErr: true,
		},
		{
			name:    "http endpoint missing host",
			in:      "github=https://",
			wantErr: true,
		},
		{
			name:    "http endpoint carries credentials",
			in:      "github=https://user:secret@mcp.example/",
			wantErr: true,
		},
		{
			name:    "stdio empty command",
			in:      "fs=stdio:",
			wantErr: true,
		},
		{
			name:    "duplicate name",
			in:      "shared=https://one.example, shared=stdio:other-server",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMCPServers(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestParseMCPServers_AuthFromEnv covers the per-server bearer-token env:
// FLAME_MCP_<NAME>_TOKEN populates an HTTP server's Authorization (as a Bearer
// header), the name is normalized (upper-cased, '-' → '_'), and stdio servers
// never pick one up.
func TestParseMCPServers_AuthFromEnv(t *testing.T) {
	t.Setenv("FLAME_MCP_GH_TOKEN", "ghp_secret")
	t.Setenv("FLAME_MCP_MY_API_TOKEN", "tok2") // for server "my-api"

	got, err := parseMCPServers("gh=https://mcp.github.com/,my-api=https://api.example.com/,local=stdio:echo hi")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byName := map[string]MCPServer{}
	for _, s := range got {
		byName[s.Name] = s
	}
	if got := byName["gh"].Authorization; got != "Bearer ghp_secret" {
		t.Errorf("gh Authorization = %q, want %q", got, "Bearer ghp_secret")
	}
	if got := byName["my-api"].Authorization; got != "Bearer tok2" {
		t.Errorf("my-api Authorization = %q, want %q (dash should normalize to underscore)", got, "Bearer tok2")
	}
	if got := byName["local"].Authorization; got != "" {
		t.Errorf("stdio server picked up Authorization %q, want none", got)
	}
}

func TestParseMCPServersRejectsAmbiguousTokenEnvironment(t *testing.T) {
	t.Setenv("FLAME_MCP_MY_API_TOKEN", "shared-secret")

	_, err := parseMCPServers("my-api=https://one.example/mcp,my.api=https://two.example/mcp")
	if err == nil {
		t.Fatal("parse err = nil, want credential-variable collision")
	}
	for _, detail := range []string{"my-api", "my.api", "FLAME_MCP_MY_API_TOKEN"} {
		if !strings.Contains(err.Error(), detail) {
			t.Fatalf("parse err = %q, want detail %q", err, detail)
		}
	}
}

func TestParseMCPServersDoesNotEchoRejectedEndpoint(t *testing.T) {
	const secret = "mcp-url-secret"
	_, err := parseMCPServers("github=https://user:" + secret + "@mcp.example")
	if err == nil {
		t.Fatal("parseMCPServers error = nil, want rejected URL credentials")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("parseMCPServers error leaked URL credential: %q", err)
	}
}
