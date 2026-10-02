package toolset

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
)

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
	server := mcpserver.Server{Name: testMCPServerName("files"), Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: "https://one.example/mcp", Authorization: "Bearer first"}
	if err := registry.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	ref := testMCPRef(server.Name, testRemoteToolName("read"))
	store := sqlite.NewApprovalRuleStore(db)
	policy, err := approvals.NewRuntimePolicy(approval.ModeSafe, store, sqlite.NewPermissionModeStore(db), NewAuthorities(registry, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.SetRule(t.Context(), ref, approval.ScopeGlobal, "", "", approval.Subject{Type: approval.SubjectAll}, approval.Allow); err != nil {
		t.Fatal(err)
	}
	original := server.AuthorityFingerprint()
	check := func(fingerprint string, wantMatch, wantStale bool) {
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
	if err := policy.SetRule(t.Context(), ref, approval.ScopeGlobal, "", "", approval.Subject{Type: approval.SubjectAll}, approval.Allow); err != nil {
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
	modes := sqlite.NewPermissionModeStore(db)
	agent := A2AAgentConfig{Name: "research", CardURL: "https://one.example/card", AllowedRPCOrigins: []string{"https://rpc.example"}}
	ref, err := tool.A2A(agent.Name)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := approvals.NewRuntimePolicy(approval.ModeSafe, store, modes, NewAuthorities(nil, []A2AAgentConfig{agent}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.SetRule(t.Context(), ref, approval.ScopeGlobal, "", "", approval.Subject{Type: approval.SubjectAll}, approval.Allow); err != nil {
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
