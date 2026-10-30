package toolset

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/qimu/integrity"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
)

func TestAuthorityReadSharesOneSourceObservationAndRefreshesNextRequest(t *testing.T) {
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("files"), Enabled: true, Connection: mcpserver.HTTPConnection{URL: "https://first.example/mcp"}}
	changed := server.Clone()
	changed.Connection = mcpserver.HTTPConnection{URL: "https://second.example/mcp"}
	registry := &authorityReadSequence{states: []mcpserver.Server{server, changed}}
	authorities := NewAuthorities(registry.Get)
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
	process := mcpserver.StdioConnection{Command: "server", Args: []string{"--stdio"}, Dir: "/workspace"}
	original := mcpserver.Server{Connection: process}.AuthorityFingerprint()
	rotated := process
	rotated.Env = map[string]string{"TOKEN": "rotated"}
	if (mcpserver.Server{Connection: rotated}).AuthorityFingerprint() != original {
		t.Fatal("credential rotation changed stdio authority")
	}
	for _, mutate := range []func(*mcpserver.StdioConnection){
		func(s *mcpserver.StdioConnection) { s.Command = "other" },
		func(s *mcpserver.StdioConnection) { s.Args = []string{"--other"} },
		func(s *mcpserver.StdioConnection) { s.Dir = "/other" },
	} {
		changed := process
		mutate(&changed)
		if (mcpserver.Server{Connection: changed}).AuthorityFingerprint() == original {
			t.Fatal("changed stdio endpoint retained authority")
		}
	}
}

func TestStandingRulesFollowCurrentSourceAuthority(t *testing.T) {
	db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "flame.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	registry := db.MCPServers
	server := mcpserver.Server{Source: mcpserver.UserSource(), Name: testsupport.ServerName("files"), Enabled: true, Connection: mcpserver.HTTPConnection{URL: "https://one.example/mcp", Authorization: "Bearer first"}}
	if err := registry.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	ref := testMCPRef(server.Name, testsupport.RemoteToolName("read"))
	store := db.ApprovalRules
	policy, err := approvals.NewRuntimePolicy(approval.ModeSafe, store, db.ApprovalModes, db.Sessions, NewAuthorities(userDefinitions(registry)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.SetRule(t.Context(), approvals.RuleChange{Tool: ref, Scope: approval.ScopeGlobal, Subject: approval.Subject{Type: approval.SubjectAll}, Decision: approval.Allow}); err != nil {
		t.Fatal(err)
	}
	original := server.AuthorityFingerprint()
	check := func(fingerprint integrity.Digest, wantMatch, wantStale bool) {
		t.Helper()
		decision, matched, err := policy.Decide(t.Context(), approval.Query{Tool: ref, SourceFingerprint: fingerprint})
		if err != nil || matched != wantMatch || (matched && decision != approval.Allow) {
			t.Fatalf("decision = %v, %v, %v", decision, matched, err)
		}
		rules, err := policy.ListRules(t.Context(), "")
		if err != nil || len(rules) != 1 || rules[0].Stale != wantStale {
			t.Fatalf("rules = %+v, %v", rules, err)
		}
	}
	check(original, true, false)
	rotated := server.Connection.(mcpserver.HTTPConnection)
	rotated.Authorization = "Bearer rotated"
	rotated.Headers = map[string]string{"X-API-Key": "rotated"}
	server.Connection = rotated
	if err := registry.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	check(server.AuthorityFingerprint(), true, false)
	moved := rotated
	moved.URL = "https://two.example/mcp"
	server.Connection = moved
	if err := registry.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	check(server.AuthorityFingerprint(), false, true)
	check(original, false, true)
	if _, err := policy.PrepareRule(t.Context(), approval.RememberRequest{Subject: approval.Subject{Type: approval.SubjectAll}, Tool: ref, SourceFingerprint: original, Scope: approval.ScopeGlobal, Decision: approval.Allow}); !errors.Is(err, approval.ErrSourceAuthorityChanged) {
		t.Fatalf("obsolete connection prepare error = %v", err)
	}
	if err := policy.SetRule(t.Context(), approvals.RuleChange{Tool: ref, Scope: approval.ScopeGlobal, Subject: approval.Subject{Type: approval.SubjectAll}, Decision: approval.Allow}); err != nil {
		t.Fatal(err)
	}
	check(server.AuthorityFingerprint(), true, false)
	if err := registry.Remove(t.Context(), server.Name); err != nil {
		t.Fatal(err)
	}
	if err := registry.Save(t.Context(), server); err != nil {
		t.Fatal(err)
	}
	rules, err := policy.ListRules(t.Context(), "")
	if err != nil || len(rules) != 0 {
		t.Fatalf("recreated source inherited rules: %v, %v", rules, err)
	}
}

// userDefinitions reads only the user registry, which is every source these
// tests create.
func userDefinitions(registry *persistence.MCPServerStore) func(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	return func(ctx context.Context, id mcpserver.ID) (mcpserver.Server, bool, error) {
		if id.Origin().Kind() != mcpserver.OriginUser {
			return mcpserver.Server{}, false, nil
		}
		return registry.Get(ctx, id.Name())
	}
}
