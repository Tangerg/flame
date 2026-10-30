package hooks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/hookprovider"
	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
)

type sourceFixture struct {
	rules      []hook.Hook
	absent     bool
	failure    error
	closeError error
	closed     int
	queries    []hookprovider.Discovery
	read       func(context.Context, hookprovider.Discovery) ([]hook.Hook, error)
}

func (s *sourceFixture) AcquireHooks(context.Context) (Use, error) {
	if s.absent {
		return nil, nil
	}
	return &sourceUseFixture{source: s}, nil
}

type sourceUseFixture struct {
	source *sourceFixture
	closed bool
}

func (u *sourceUseFixture) Discover(ctx context.Context, q hookprovider.Discovery) ([]hook.Hook, error) {
	s := u.source
	s.queries = append(s.queries, q)
	if s.read != nil {
		return s.read(ctx, q)
	}
	result := slices.Clone(s.rules)
	if !q.IncludeProject {
		result = slices.DeleteFunc(result, func(h hook.Hook) bool { return h.Scope == hook.ScopeProject })
	}
	return result, s.failure
}
func (*sourceUseFixture) RunHookCommand(context.Context, apphooks.CommandRequest) apphooks.CommandResult {
	return apphooks.CommandResult{Decision: hook.CommandDecision{Verdict: hook.CommandAllow}}
}
func (u *sourceUseFixture) Close() error {
	if !u.closed {
		u.closed = true
		u.source.closed++
	}
	return u.source.closeError
}

func TestResolverRechecksTrustAndRetainsOneSourceForItsEvents(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(root, "nested")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	source := &sourceFixture{rules: []hook.Hook{
		{Event: hook.SessionStart, Inject: "global", Scope: hook.ScopeGlobal, Source: filepath.Join(home, ".flame", "hooks.json")},
		{Event: hook.SessionStart, Inject: "project", Scope: hook.ScopeProject, Source: filepath.Join(root, ".flame", "hooks.json")},
	}}
	trusted := false
	resolver, err := NewResolver(home, func(_ context.Context, project string) (bool, error) { return project == root && trusted, nil }, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := resolver.For(t.Context(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	trusted = true
	after, err := resolver.For(t.Context(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	source.rules[0].Inject = "changed"
	if got := before.Run(t.Context(), hook.Input{Event: hook.SessionStart}).InjectContext; got != "global" {
		t.Fatalf("original rules=%q", got)
	}
	if got := after.Run(t.Context(), hook.Input{Event: hook.SessionStart}).InjectContext; got != "global\nproject" {
		t.Fatalf("trusted rules=%q", got)
	}
	if source.closed != 0 || len(source.queries) != 2 || source.queries[0].IncludeProject || !source.queries[1].IncludeProject || source.queries[0].ProjectRoot != root {
		t.Fatalf("source lifetime and trust=%+v", source)
	}
	if err := errors.Join(before.Close(), after.Close()); err != nil {
		t.Fatal(err)
	}
	if source.closed != 2 {
		t.Fatal("event sources were not released")
	}
}

func TestResolverInspectionClosesAndTransfersTheSourceSnapshot(t *testing.T) {
	cwd := t.TempDir()
	source := &sourceFixture{rules: []hook.Hook{{Event: hook.Stop, Inject: "original", Scope: hook.ScopeProject, Source: filepath.Join(cwd, ".flame", "hooks.json")}}}
	resolver, err := NewResolver("", nil, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := resolver.Inspect(t.Context(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	first.Hooks[0].Inject = "caller mutation"
	second, err := resolver.Inspect(t.Context(), cwd)
	if err != nil {
		t.Fatal(err)
	}
	if second.Hooks[0].Inject != "original" || source.closed != 2 || !source.queries[0].IncludeProject || second.ProjectTrusted {
		t.Fatalf("inspection=%+v, source=%+v", second, source)
	}
}

func TestResolverPreservesTrustAndSourceFailures(t *testing.T) {
	cwd := t.TempDir()
	trustFailure := errors.New("trust unavailable")
	source := &sourceFixture{}
	resolver, err := NewResolver("", func(context.Context, string) (bool, error) { return false, trustFailure }, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.For(t.Context(), cwd); !errors.Is(err, trustFailure) || len(source.queries) != 0 {
		t.Fatalf("trust failure=%v", err)
	}
	readFailure, closeFailure := errors.New("source unavailable"), errors.New("source close failed")
	source.failure, source.closeError = readFailure, closeFailure
	resolver, err = NewResolver("", nil, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.For(t.Context(), cwd); !errors.Is(err, readFailure) || !errors.Is(err, closeFailure) || source.closed != 1 {
		t.Fatalf("source failure=%v", err)
	}
}

func TestResolverPassesCallerContextAndRefusesUntrustedProjectContributions(t *testing.T) {
	type key struct{}
	ctx := context.WithValue(t.Context(), key{}, "original")
	cwd := t.TempDir()
	source := &sourceFixture{read: func(got context.Context, q hookprovider.Discovery) ([]hook.Hook, error) {
		if got.Value(key{}) != "original" || q.IncludeProject {
			t.Error("discovery lost the caller's trust input")
		}
		return []hook.Hook{{Event: hook.Stop, Inject: "untrusted", Scope: hook.ScopeProject, Source: filepath.Join(cwd, ".flame", "hooks.json")}}, nil
	}}
	resolver, err := NewResolver("", nil, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.For(ctx, cwd); err == nil || source.closed != 1 {
		t.Fatalf("untrusted contribution=%v", err)
	}
}

func TestResolverAbsenceLeavesTrustManagementAvailable(t *testing.T) {
	cwd := t.TempDir()
	source := &sourceFixture{absent: true}
	resolver, err := NewResolver("", func(context.Context, string) (bool, error) { return true, nil }, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := resolver.For(t.Context(), cwd)
	if err != nil || bound != nil {
		t.Fatalf("absent source=%v, %v", bound, err)
	}
	inspected, err := resolver.Inspect(t.Context(), cwd)
	if err != nil || inspected.ProjectRoot != cwd || !inspected.ProjectTrusted || len(inspected.Hooks) != 0 {
		t.Fatalf("trust management=%+v, %v", inspected, err)
	}
}

func TestResolverRequiresAnExplicitSource(t *testing.T) {
	for _, source := range []Source{nil, (*sourceFixture)(nil)} {
		if _, err := NewResolver("", nil, source, nil); err == nil {
			t.Fatal("missing source was accepted")
		}
	}
}
