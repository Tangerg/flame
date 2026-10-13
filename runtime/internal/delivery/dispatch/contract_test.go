package dispatch

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
	"github.com/Tangerg/flame/runtime/protocol"
)

// capabilityRuntime is a Runtime that only answers discovery — enough to drive
// the gate, since that is the only thing the gate reads.
type capabilityRuntime struct {
	features map[string]bool
}

func newOperationEndpoint(t *testing.T, target any) *delivery.Endpoint {
	t.Helper()
	endpoint, err := delivery.NewEndpoint(target, delivery.EndpointConfig{Lifetime: t.Context()})
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func newTestRouter(t *testing.T, target any) *Router {
	t.Helper()
	router, err := New(newOperationEndpoint(t, target))
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func (c *capabilityRuntime) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	advertised := make(map[string]protocol.FeatureCapability, len(c.features))
	for name, enabled := range c.features {
		published, _ := protocol.LookupFeature(name)
		advertised[name] = protocol.FeatureCapability{
			Enabled:               enabled,
			ClientOptIn:           published.ClientOptIn,
			RequiredByRunProtocol: published.RequiredByRunProtocol,
		}
	}
	return &protocol.DiscoverResponse{Capabilities: protocol.ServerCapabilities{Features: advertised}}, nil
}

func (c *capabilityRuntime) SubscribeRuntime(context.Context, protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error) {
	return &protocol.RuntimeSubscribeResponse{}, func(func(protocol.RuntimeEvent, error) bool) {}, nil
}

func (c *capabilityRuntime) ListAgentMemory(context.Context, protocol.AgentMemoryListRequest) (*protocol.Page[protocol.AgentMemoryItem], error) {
	return &protocol.Page[protocol.AgentMemoryItem]{Data: []protocol.AgentMemoryItem{}}, nil
}

func (c *capabilityRuntime) UsageSummary(context.Context, protocol.UsageSummaryRequest) (*protocol.UsageSummary, error) {
	return &protocol.UsageSummary{}, nil
}

func TestRetiredBusinessViewMethodsAreUnknown(t *testing.T) {
	for _, method := range []string{"plugins.readView", "plugins.readSchedules"} {
		response := call(t, map[string]bool{protocol.FeaturePlugins: true}, method, `{}`)
		failure, ok := response.Error.(*transport.Error)
		if !ok || failure.Code != -32601 {
			t.Fatalf("retired %s remains callable: %+v", method, response)
		}
	}
}

func TestUsageSummaryIsIndependentOfPluginPresentation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		response := call(t, map[string]bool{protocol.FeaturePlugins: enabled}, "usage.summary", `{"sinceDays":7}`)
		if response.Error != nil {
			t.Fatalf("canonical usage with plugin presentation=%t: %+v", enabled, response.Error)
		}
		retired := call(t, map[string]bool{protocol.FeaturePlugins: enabled}, "plugins.readUsage", `{}`)
		failure, ok := retired.Error.(*transport.Error)
		if !ok || failure.Code != -32601 {
			t.Fatalf("retired usage entrance survived: %+v", retired.Error)
		}
	}
}

func (c *capabilityRuntime) ListRuns(context.Context, protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
	return protocol.NewPage([]protocol.RunRef{}), nil
}

func (c *capabilityRuntime) RollbackSession(context.Context, protocol.RollbackSessionRequest) (*protocol.RollbackSessionResponse, error) {
	return &protocol.RollbackSessionResponse{DroppedRuns: []protocol.DroppedRun{}}, nil
}

func call(t *testing.T, features map[string]bool, method, params string) *transport.Response {
	t.Helper()
	d := newTestRouter(t, &capabilityRuntime{features: features})
	res := d.Dispatch(t.Context(), &transport.Request{
		ID: testID("1"), Method: method, Params: jsontext.Value(params),
	})
	if res.Response == nil {
		t.Fatalf("%s returned no response", method)
	}
	return res.Response
}

func problemType(t *testing.T, resp *transport.Response) string {
	t.Helper()
	if resp.Error == nil {
		return ""
	}
	rpcErr, ok := resp.Error.(*transport.Error)
	if !ok {
		t.Fatalf("response error is %T, want *transport.Error", resp.Error)
	}
	var problem protocol.ProblemData
	if err := json.Unmarshal(rpcErr.Data, &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return problem.Type
}

// TestCapabilityGateRefusesADisabledFeature covers the unconditional form: with
// the feature off the method is refused before the use case runs, and with it on
// the call goes through. This is the rule the 20 hand-written handler checks used
// to state — one per method, each able to drift from what discovery advertised.
func TestCapabilityGateRefusesADisabledFeature(t *testing.T) {
	t.Parallel()

	const params = `{"scope":"project","workspace":{"path":"/workspace"}}`
	off := call(t, map[string]bool{"agentMemory": false}, "agentMemory.list", params)
	if got := problemType(t, off); got != "capability_not_negotiated" {
		t.Fatalf("agentMemory.list with the feature off = %q, want capability_not_negotiated", got)
	}
	on := call(t, map[string]bool{"agentMemory": true}, "agentMemory.list", params)
	if on.Error != nil {
		t.Fatalf("agentMemory.list with the feature on: %+v", on.Error)
	}
}

// TestCapabilityGateOnlyBitesTheGatedRequest covers the conditional form: a
// method stays usable in its default shape while one option is gated. That is why
// When exists — an unconditional rule would take the whole method away.
func TestCapabilityGateOnlyBitesTheGatedRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		method   string
		params   string
		features map[string]bool
		want     string
	}{{
		name:   "subscribing without watches needs no fileWatch",
		method: "runtime.subscribe", params: `{"topics":["skills.changed"]}`,
		features: map[string]bool{"fileWatch": false},
		want:     "",
	}, {
		name:   "an empty watch list is not a watch",
		method: "runtime.subscribe", params: `{"topics":["skills.changed"],"watches":[]}`,
		features: map[string]bool{"fileWatch": false},
		want:     "",
	}, {
		name:   "registering a watch needs fileWatch",
		method: "runtime.subscribe", params: `{"topics":["files.changed"],"watches":[{"watchId":"w1","workspace":{"path":"/workspace"}}]}`,
		features: map[string]bool{"fileWatch": false},
		want:     "capability_not_negotiated",
	}, {
		name:   "a history rollback needs no checkpoints",
		method: "sessions.rollback", params: `{"sessionId":"ses_1"}`,
		features: map[string]bool{"checkpoints": false},
		want:     "",
	}, {
		name:   "restoring files needs checkpoints",
		method: "sessions.rollback", params: `{"sessionId":"ses_1","toRunId":"run_1","restoreType":"files"}`,
		features: map[string]bool{"checkpoints": false},
		want:     "capability_not_negotiated",
	}, {
		name:   "restoring both needs checkpoints",
		method: "sessions.rollback", params: `{"sessionId":"ses_1","toRunId":"run_1","restoreType":"both"}`,
		features: map[string]bool{"checkpoints": false},
		want:     "capability_not_negotiated",
	}, {
		name:   "restoring files is allowed once checkpoints are on",
		method: "sessions.rollback", params: `{"sessionId":"ses_1","toRunId":"run_1","restoreType":"files"}`,
		features: map[string]bool{"checkpoints": true},
		want:     "",
	}, {
		name:   "listing root runs needs no subagents",
		method: "runs.list", params: `{}`,
		features: map[string]bool{"subagents": false},
		want:     "",
	}, {
		// The whole point of the rule: the contract forbids reading an explicit true
		// as false, because the page would come back looking complete.
		name:   "asking for descendants needs subagents",
		method: "runs.list", params: `{"includeDescendants":true}`,
		features: map[string]bool{"subagents": false},
		want:     "capability_not_negotiated",
	}, {
		name:   "server support does not replace client opt-in",
		method: "runs.list", params: `{"includeDescendants":true}`,
		features: map[string]bool{"subagents": true},
		want:     "capability_not_negotiated",
	}, {
		name:   "client opt-in and server support authorize descendants",
		method: "runs.list",
		params: `{
			"includeDescendants": true,
			"_meta": {
				"clientCapabilities": {
					"features": {"subagents": {"enabled": true}}
				}
			}
		}`,
		features: map[string]bool{"subagents": true},
		want:     "",
	}, {
		name:   "an explicit false is not a request for descendants",
		method: "runs.list", params: `{"includeDescendants":false}`,
		features: map[string]bool{"subagents": false},
		want:     "",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := call(t, tt.features, tt.method, tt.params)
			if got := problemType(t, resp); got != tt.want {
				t.Fatalf("problem type = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestUnknownMethodStaysUnknown keeps the registry from becoming permissive: a
// name nobody registered is method_not_found, not a nil-handler panic.
func TestUnknownMethodStaysUnknown(t *testing.T) {
	t.Parallel()

	resp := call(t, nil, "runs.teleport", `{}`)
	if got := problemType(t, resp); got != "method_not_found" {
		t.Fatalf("problem type = %q, want method_not_found", got)
	}
}

func (c *capabilityRuntime) GetWorkspaceDiff(context.Context, protocol.GetDiffRequest) (*protocol.Diff, error) {
	return &protocol.Diff{Baseline: protocol.DiffBaseline{Type: protocol.DiffBaselineEmptyTree}}, nil
}
func TestCanonicalDiffRequiresGitAndRetiresPluginWrapper(t *testing.T) {
	const params = `{"workspace":{"path":"/runtime"},"format":"rows"}`
	for _, features := range []map[string]bool{{protocol.FeaturePlugins: false, protocol.FeatureGit: true}, {protocol.FeaturePlugins: true, protocol.FeatureGit: false}, {protocol.FeaturePlugins: true, protocol.FeatureGit: true}} {
		response := call(t, features, "workspace.diff.get", params)
		if features[protocol.FeatureGit] {
			if response.Error != nil {
				t.Fatalf("canonical diff: %+v", response.Error)
			}
		} else if got := problemType(t, response); got != "capability_not_negotiated" {
			t.Fatalf("missing Git capability: %s", got)
		}
		if got := problemType(t, call(t, features, "plugins.readDiff", `{}`)); got != "method_not_found" {
			t.Fatalf("retired diff entrance: %s", got)
		}
	}
}

func (c *capabilityRuntime) ListDiscoveredSkills(context.Context, protocol.WorkspaceQuery) (*protocol.SkillDiscovery, error) {
	return &protocol.SkillDiscovery{Skills: []protocol.Skill{}, Diagnostics: []protocol.SkillDiagnostic{}}, nil
}
func (c *capabilityRuntime) GetDiscoveredSkill(context.Context, protocol.SkillDetailRequest) (*protocol.SkillDetail, error) {
	return &protocol.SkillDetail{Skill: protocol.Skill{Name: "verify", Scope: protocol.SkillScopeUser}, Path: "/runtime/verify/SKILL.md", Revision: strings.Repeat("1", 64), Instructions: "Verify"}, nil
}
func (c *capabilityRuntime) ListManagedSkills(context.Context) (*protocol.Page[protocol.ManagedSkill], error) {
	return protocol.NewPage([]protocol.ManagedSkill{}), nil
}
func (c *capabilityRuntime) ListSkillProposals(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.SkillProposal], error) {
	return protocol.NewPage([]protocol.SkillProposal{}), nil
}
func TestCanonicalSkillReadsAreIndependentOfPluginPresentation(t *testing.T) {
	for _, entry := range []struct{ method, params, retired string }{
		{"skills.discovered.list", `{"workspace":{"path":"/runtime"}}`, "plugins.readSkills"},
		{"skills.discovered.get", `{"workspace":{"path":"/runtime"},"name":"verify"}`, "plugins.readSkill"},
		{"skills.library.list", `{}`, "plugins.readSkillLibrary"},
		{"skills.proposals.list", `{"workspace":{"path":"/runtime"}}`, "plugins.readSkillProposals"},
	} {
		response := call(t, map[string]bool{protocol.FeatureSkills: true, protocol.FeaturePlugins: false}, entry.method, entry.params)
		if response.Error != nil {
			t.Fatalf("canonical Skill read depends on presentation: %v", response.Error)
		}
		if got := problemType(t, call(t, map[string]bool{protocol.FeatureSkills: false}, entry.method, entry.params)); got != "capability_not_negotiated" {
			t.Fatalf("Skill capability gate: %s", got)
		}
		if got := problemType(t, call(t, map[string]bool{protocol.FeatureSkills: true}, entry.retired, `{}`)); got != "method_not_found" {
			t.Fatalf("retired Skill wrapper: %s", got)
		}
	}
}
