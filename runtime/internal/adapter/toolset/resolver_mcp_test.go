package toolset

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	identitytool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type mcpToolStub struct {
	name   string
	server string
	remote string
}

func (m mcpToolStub) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: m.name, InputSchema: []byte(`{"type":"object"}`)}
}

func (mcpToolStub) Call(context.Context, toolcontract.Invocation) (chat.ToolOutput, error) {
	return chat.ToolOutput{}, nil
}

func (m mcpToolStub) MCPToolIdentity() (string, string) { return m.server, m.remote }

// A stub's server is a user server; installation tools override the source.
func (m mcpToolStub) SourceConfig() mcp.ServerConfig {
	return mcp.ServerConfig{Source: mcpserver.UserSource(), Name: testsupport.ServerName(m.server)}
}

func TestResolverMCPToolsReadsCurrentPolicy(t *testing.T) {
	disabled := map[identitytool.Ref]bool{}
	resolver := &Resolver{mcpToolDisabled: func(ref identitytool.Ref) bool { return disabled[ref] }}
	resolver.SetMCPTools([]toolcontract.Tool{
		wrappedMCPTool(mcpToolStub{name: "files_read", server: "files", remote: "read"}),
		wrappedMCPTool(mcpToolStub{name: "files_write", server: "files", remote: "write"}),
	})

	tests := []struct {
		name     string
		disabled map[identitytool.Ref]bool
		want     []string
	}{
		{name: "no disabled tools", disabled: map[identitytool.Ref]bool{}, want: []string{"files_read", "files_write"}},
		{
			name:     "policy update hides tool",
			disabled: map[identitytool.Ref]bool{testMCPRef(testsupport.ServerName("files"), testsupport.RemoteToolName("write")): true},
			want:     []string{"files_read"},
		},
		{name: "later policy restores tool", disabled: map[identitytool.Ref]bool{}, want: []string{"files_read", "files_write"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disabled = tt.disabled
			gotTools, err := resolver.mcpTools()
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(gotTools))
			for i, tool := range gotTools {
				got[i] = tool.Definition().Name
			}
			if len(got) != len(tt.want) {
				t.Fatalf("tools = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("tools = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func wrappedMCPTool(inner toolcontract.Tool) toolcontract.Tool {
	return decorateCall(inner, inner.Call)
}

func TestResolverMCPPolicyUsesSourceIdentityNotModelName(t *testing.T) {
	disabledRef := testMCPRef(testsupport.ServerName("a_b"), testsupport.RemoteToolName("c"))
	liveRef := testMCPRef(testsupport.ServerName("a"), testsupport.RemoteToolName("b_c"))
	disabledName := disabledRef.ModelName()
	liveName := liveRef.ModelName()
	if disabledName != liveName {
		t.Fatalf("fixture names do not collide: %q != %q", disabledName, liveName)
	}

	resolver := &Resolver{mcpToolDisabled: func(ref identitytool.Ref) bool { return ref == disabledRef }}
	resolver.SetMCPTools([]toolcontract.Tool{mcpToolStub{
		name: liveName, server: "a", remote: "b_c",
	}})

	got, err := resolver.mcpTools()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Definition().Name != liveName {
		t.Fatalf("policy for %+v hid colliding live tool %+v", disabledRef, liveRef)
	}
}

func TestResolverMCPPolicyRejectsInvalidSourceIdentity(t *testing.T) {
	valid := mcpToolStub{name: "valid", server: "files", remote: "read"}
	cycle := &cyclicMCPTool{Tool: valid}
	for _, test := range []struct {
		name string
		tool toolcontract.Tool
		want error
	}{
		{name: "nil"},
		{name: "absent", tool: anonymousMCPTool{Tool: valid}},
		{name: "malformed", tool: mcpToolStub{name: "invalid"}},
		{name: "cyclic decorator", tool: cycle, want: toolcontract.ErrInvalidWrappingChain},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Identity is required even when no disabled-tool policy is installed.
			resolver := &Resolver{}
			resolver.SetMCPTools([]toolcontract.Tool{valid, test.tool})
			got, err := resolver.mcpTools()
			if err == nil || got != nil || test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("MCP catalog = %v, %v; want complete rejection", got, err)
			}
		})
	}
}

func TestDiscoveryRejectsBrokenMCPCapabilities(t *testing.T) {
	for _, executable := range []toolcontract.Tool{
		mcpToolStub{name: "invalid", server: "files"},
		&cyclicMCPTool{Tool: mcpToolStub{name: "cyclic"}},
	} {
		if search, err := NewDiscovery([]toolcontract.Tool{executable}); err == nil || search != nil {
			t.Fatalf("discovery = %v, %v; malformed MCP capability must not become built-in", search, err)
		}
	}
}

type anonymousMCPTool struct{ toolcontract.Tool }

type cyclicMCPTool struct{ toolcontract.Tool }

func (c *cyclicMCPTool) Unwrap() toolcontract.Tool { return c }

func TestResolverSetMCPToolsSnapshotsInput(t *testing.T) {
	resolver := &Resolver{}
	tools := []toolcontract.Tool{mcpToolStub{name: "before", server: "files", remote: "read"}}
	resolver.SetMCPTools(tools)
	tools[0] = mcpToolStub{name: "after"}

	got, err := resolver.mcpTools()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Definition().Name != "before" {
		t.Fatalf("mcp tools retained caller-owned slice: %v", got)
	}
}
