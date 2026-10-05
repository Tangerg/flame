package execution

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type identifiedPolicyTool struct {
	toolcontract.Tool
	server, remote string
}

func (i identifiedPolicyTool) MCPToolIdentity() (string, string) { return i.server, i.remote }
func (i identifiedPolicyTool) SourceFingerprint() fingerprint.Digest {
	return testsupport.ToolFingerprint(tool.Ref{})
}

func (i identifiedPolicyTool) SourceConfig() mcp.ServerConfig {
	name, _ := mcpserver.ParseServerName(i.server)
	return mcp.ServerConfig{Source: mcpserver.UserSource(), Name: name}
}

type decoratedPolicyTool struct{ toolcontract.Tool }

func (d decoratedPolicyTool) Unwrap() toolcontract.Tool { return d.Tool }

func TestMCPApprovalUsesScopeCapabilityIdentity(t *testing.T) {
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "source_original"}, func(context.Context, struct{}) (string, error) { return "done", nil })
	if err != nil {
		t.Fatal(err)
	}
	identified := identifiedPolicyTool{Tool: executable, server: "source", remote: "original"}
	for _, executable := range []toolcontract.Tool{identified, decoratedPolicyTool{Tool: decoratedPolicyTool{Tool: identified}}} {
		ref, err := toolset.Identify(executable)
		if err != nil {
			t.Fatal(err)
		}
		fingerprint, err := toolset.SourceFingerprint(executable, ref)
		if err != nil {
			t.Fatal(err)
		}
		observed := &observedInteractionTool{inner: executable, ref: ref, sourceFingerprint: fingerprint, interpreter: testInteractionToolInterpreter{}, start: interactionTestStart(), session: &interactionSession{}}
		request, err := observed.authorizationRequest("call", "source_original", tool.Arguments{}, false)
		if err != nil || request.Tool != ref || request.SourceFingerprint != fingerprint {
			t.Fatalf("request = %+v, %v", request, err)
		}
		if server, remote, _ := request.Tool.MCP(); server != testsupport.UserMCPServer("source") || remote.String() != "original" {
			t.Fatalf("identity = %v", request.Tool)
		}
	}
	for _, malformed := range []toolcontract.Tool{executable, identifiedPolicyTool{Tool: executable, server: "source"}, &cyclicPolicyTool{Tool: executable}} {
		if _, err := toolset.Identify(malformed); err == nil {
			t.Fatal("accepted executable without valid source identity")
		}
	}
}
