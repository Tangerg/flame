package toolset

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"

	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

func TestAuthorityReadSharesOneSourceObservationAndRefreshesNextRequest(t *testing.T) {
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("files"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: "https://first.example/mcp"}
	changed := server.Clone()
	changed.URL = "https://second.example/mcp"
	registry := &authorityReadSequence{states: []mcpserver.Server{server, changed}}
	authorities := NewAuthorities(registry.Get, nil)
	first := testMCPRef(server.Name, testsupport.RemoteToolName("first"))
	second := testMCPRef(server.Name, testsupport.RemoteToolName("second"))
	refs := []tool.Ref{first, second}
	observed, err := authorities.Fingerprints(t.Context(), refs)
	if err != nil || observed[first] != server.AuthorityFingerprint() || observed[second] != observed[first] {
		t.Fatalf("same-source rule projection disagreed: %v", err)
	}
	refreshed, err := authorities.Fingerprints(t.Context(), refs)
	if err != nil || refreshed[first] != changed.AuthorityFingerprint() || refreshed[second] != refreshed[first] {
		t.Fatalf("next rule projection retained superseded authority: %v", err)
	}
	if _, _, err := authorities.Fingerprint(t.Context(), tool.Ref{}); err == nil {
		t.Fatal("singular dispatch admitted an unconstructed source")
	}
	if _, err := authorities.Fingerprints(t.Context(), []tool.Ref{first, {}}); err == nil {
		t.Fatal("rule projection admitted an unconstructed source")
	}
}

type authorityReadSequence struct{ states []mcpserver.Server }

func (r *authorityReadSequence) Get(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	server := r.states[0].Clone()
	if len(r.states) > 1 {
		r.states = r.states[1:]
	}
	return server, true, nil
}

func TestAuthorityFingerprintsTrackEndpointsAndExcludeCredentials(t *testing.T) {
	server := mcpserver.Server{Transport: mcpserver.TransportStdio, Command: "server", Args: []string{"--stdio"}, Dir: "/workspace"}
	original := server.AuthorityFingerprint()
	server.Env = map[string]string{"TOKEN": "rotated"}
	if server.AuthorityFingerprint() != original {
		t.Fatal("credential rotation changed stdio authority")
	}
	for _, mutate := range []func(*mcpserver.Server){
		func(s *mcpserver.Server) { s.Command = "other" },
		func(s *mcpserver.Server) { s.Args = []string{"--other"} },
		func(s *mcpserver.Server) { s.Dir = "/other" },
	} {
		changed := server
		mutate(&changed)
		if changed.AuthorityFingerprint() == original {
			t.Fatal("changed stdio endpoint retained authority")
		}
	}
	agent := A2AAgentConfig{CardURL: "https://card.example", AllowedRPCOrigins: []string{"https://one.example", "https://two.example"}}
	original = agent.AuthorityFingerprint()
	slices.Reverse(agent.AllowedRPCOrigins)
	agent.AllowedRPCOrigins = append(agent.AllowedRPCOrigins, agent.AllowedRPCOrigins[0])
	if agent.AuthorityFingerprint() != original {
		t.Fatal("origin set order changed authority")
	}
	agent.AllowedRPCOrigins = append(agent.AllowedRPCOrigins, "https://other.example")
	if agent.AuthorityFingerprint() == original {
		t.Fatal("added RPC origin retained authority")
	}
}

func TestStandingRulesFollowCurrentSourceAuthority(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	registry := sqlite.NewMCPServerStore(db)
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("files"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: "https://one.example/mcp", Authorization: "Bearer first"}
	if err := registry.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	ref := testMCPRef(server.Name, testsupport.RemoteToolName("read"))
	store := sqlite.NewApprovalRuleStore(db)
	policy, err := approvals.NewRuntimePolicy(approval.ModeSafe, store, sqlite.NewModeStore(db), NewAuthorities(userDefinitions(registry), nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.SetRule(t.Context(), approvals.RuleChange{Tool: ref, Scope: approval.ScopeGlobal, Subject: approval.Subject{Type: approval.SubjectAll}, Decision: approval.Allow}, ""); err != nil {
		t.Fatal(err)
	}
	original := server.AuthorityFingerprint()
	check := func(fingerprint fingerprint.Digest, wantMatch, wantStale bool) {
		t.Helper()
		decision, matched, err := policy.Decide(t.Context(), approval.Query{Tool: ref, SourceFingerprint: fingerprint})
		if err != nil || matched != wantMatch || (matched && decision != approval.Allow) {
			t.Fatalf("decision = %v, %v, %v", decision, matched, err)
		}
		rules, err := policy.Rules(t.Context(), "", "")
		if err != nil || len(rules) != 1 || rules[0].Stale != wantStale {
			t.Fatalf("rules = %+v, %v", rules, err)
		}
	}
	check(original, true, false)
	server.Authorization = "Bearer rotated"
	server.Headers = map[string]string{"X-API-Key": "rotated"}
	if err := registry.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	check(server.AuthorityFingerprint(), true, false)
	server.URL = "https://two.example/mcp"
	if err := registry.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	check(server.AuthorityFingerprint(), false, true)
	check(original, false, true)
	if err := policy.Remember(t.Context(), approval.RememberRequest{Subject: approval.Subject{Type: approval.SubjectAll}, Tool: ref, SourceFingerprint: original, Scope: approval.ScopeGlobal, Decision: approval.Allow}); !errors.Is(err, approval.ErrSourceAuthorityChanged) {
		t.Fatalf("obsolete connection remember error = %v", err)
	}
	if err := policy.SetRule(t.Context(), approvals.RuleChange{Tool: ref, Scope: approval.ScopeGlobal, Subject: approval.Subject{Type: approval.SubjectAll}, Decision: approval.Allow}, ""); err != nil {
		t.Fatal(err)
	}
	check(server.AuthorityFingerprint(), true, false)
	if err := registry.Remove(t.Context(), server.Name); err != nil {
		t.Fatal(err)
	}
	if err := registry.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	rules, err := policy.Rules(t.Context(), "", "")
	if err != nil || len(rules) != 0 {
		t.Fatalf("recreated source inherited rules: %v, %v", rules, err)
	}
}

func TestA2ARulesBecomeStaleWhenCardAuthorityChanges(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewApprovalRuleStore(db)
	modes := sqlite.NewModeStore(db)
	agent := A2AAgentConfig{Name: "research", CardURL: "https://one.example/card", AllowedRPCOrigins: []string{"https://rpc.example"}}
	ref, err := tool.A2A(agent.Name)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := approvals.NewRuntimePolicy(approval.ModeSafe, store, modes, NewAuthorities(nil, []A2AAgentConfig{agent}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.SetRule(t.Context(), approvals.RuleChange{Tool: ref, Scope: approval.ScopeGlobal, Subject: approval.Subject{Type: approval.SubjectAll}, Decision: approval.Allow}, ""); err != nil {
		t.Fatal(err)
	}
	agent.CardURL = "https://two.example/card"
	policy, err = approvals.NewRuntimePolicy(approval.ModeSafe, store, modes, NewAuthorities(nil, []A2AAgentConfig{agent}), nil)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := policy.Rules(t.Context(), "", "")
	if err != nil || len(rules) != 1 || !rules[0].Stale {
		t.Fatalf("rules=%v, %v", rules, err)
	}
	if _, matched, err := policy.Decide(t.Context(), approval.Query{Tool: ref, SourceFingerprint: agent.AuthorityFingerprint()}); err != nil || matched {
		t.Fatalf("new card matched old grant: %v, %v", matched, err)
	}
}

// userDefinitions reads only the user registry, which is every source these
// tests create.
func userDefinitions(registry *sqlite.MCPServerStore) func(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	return func(ctx context.Context, id mcpserver.ID) (mcpserver.Server, bool, error) {
		if id.Origin().Kind() != mcpserver.OriginUser {
			return mcpserver.Server{}, false, nil
		}
		return registry.Get(ctx, id.Name())
	}
}
