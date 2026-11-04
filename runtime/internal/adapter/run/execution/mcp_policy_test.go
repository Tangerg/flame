package execution

import (
	"context"
	"testing"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

type identifiedPolicyTool struct {
	toolcontract.Tool
	server, remote string
}

func (i identifiedPolicyTool) MCPToolIdentity() (string, string) { return i.server, i.remote }
func (i identifiedPolicyTool) SourceFingerprint() framevalue.Digest {
	return testsupport.ToolFingerprint(tool.Ref{})
}

func (i identifiedPolicyTool) SourceConfig() mcp.ServerConfig {
	name, _ := mcpserver.ParseServerName(i.server)
	return mcp.ServerConfig{Server: mcpserver.Server{Source: mcpserver.UserSource(), Name: name}}
}

func (i identifiedPolicyTool) MCPSource() (mcp.CapturedSource, bool) {
	return mcp.CapturedSource{Config: i.SourceConfig(), RemoteName: i.remote}, true
}

type decoratedPolicyTool struct {
	toolcontract.Tool
	source      mcp.CapturedSource
	fingerprint framevalue.Digest
}

func (d decoratedPolicyTool) MCPSource() (mcp.CapturedSource, bool) { return d.source, true }
func (d decoratedPolicyTool) SourceFingerprint() framevalue.Digest  { return d.fingerprint }

func TestMCPApprovalUsesCapturedSourceIdentity(t *testing.T) {
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "source_original"}, func(context.Context, struct{}) (string, error) { return "done", nil })
	if err != nil {
		t.Fatal(err)
	}
	identified := identifiedPolicyTool{Tool: executable, server: "source", remote: "original"}
	source, _ := identified.MCPSource()
	decorated := decoratedPolicyTool{Tool: identified, source: source, fingerprint: identified.SourceFingerprint()}
	for _, executable := range []toolcontract.Tool{identified, decoratedPolicyTool{Tool: decorated, source: source, fingerprint: identified.SourceFingerprint()}} {
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
	for _, malformed := range []toolcontract.Tool{executable, identifiedPolicyTool{Tool: executable, server: "source"}} {
		if _, err := toolset.Identify(malformed); err == nil {
			t.Fatal("accepted executable without valid source identity")
		}
	}
}
