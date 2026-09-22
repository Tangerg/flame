package agentexec

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type identifiedPolicyTool struct {
	toolcontract.Tool
	server, remote string
}

func (i identifiedPolicyTool) MCPToolIdentity() (string, string) { return i.server, i.remote }

type decoratedPolicyTool struct{ toolcontract.Tool }

func (d decoratedPolicyTool) Unwrap() toolcontract.Tool { return d.Tool }

func TestMCPApprovalUsesScopeCapabilityIdentity(t *testing.T) {
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "presentation_only"}, func(context.Context, struct{}) (string, error) {
		return "done", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	identified := identifiedPolicyTool{Tool: executable, server: "source", remote: "original"}
	for _, test := range []struct {
		name string
		tool toolcontract.Tool
		want bool
	}{
		{name: "plain", tool: identified, want: true},
		{name: "decorated", tool: decoratedPolicyTool{Tool: decoratedPolicyTool{Tool: identified}}, want: true},
		{name: "outer identity wins", tool: identifiedPolicyTool{Tool: decoratedPolicyTool{Tool: identified}, server: "other", remote: "original"}},
		{name: "built-in", tool: executable},
	} {
		t.Run(test.name, func(t *testing.T) {
			observed := &observedInteractionTool{
				inner: test.tool, interpreter: testInteractionToolInterpreter{}, start: interactionTestStart(),
				session: &interactionSession{mcpToolAutoApproved: func(server, remote string) bool {
					return server == "source" && remote == "original"
				}},
			}
			request, err := observed.authorizationRequest("call", "presentation_only", tool.Arguments{}, false)
			if err != nil || request.AutoApproved != test.want {
				t.Fatalf("auto-approved = %t, err = %v; want %t", request.AutoApproved, err, test.want)
			}
		})
	}
	for _, malformed := range []toolcontract.Tool{
		identifiedPolicyTool{Tool: executable, server: "source"},
		&cyclicPolicyTool{Tool: executable},
	} {
		observed := &observedInteractionTool{
			inner: malformed, interpreter: testInteractionToolInterpreter{}, start: interactionTestStart(),
			session: &interactionSession{mcpToolAutoApproved: func(string, string) bool {
				t.Fatal("invalid identity reached approval policy")
				return true
			}},
		}
		if _, err := observed.authorizationRequest("call", "presentation_only", tool.Arguments{}, false); !errors.Is(err, interaction.ErrHostFailure) {
			t.Fatalf("identity error = %v, want host failure", err)
		}
	}
}
