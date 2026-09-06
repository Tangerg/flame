package workspace

import (
	"context"
	"errors"
	"reflect"
	"testing"

	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

func TestNewHooksRequiresCompleteDependencies(t *testing.T) {
	for _, test := range []struct {
		name      string
		scope     *Scope
		inspector HookInspector
		trust     HookTrustStore
	}{
		{name: "scope", inspector: &fakeHookInspector{}, trust: &fakeHookTrust{}},
		{name: "inspector", scope: NewScope("", "", testPaths{}), trust: &fakeHookTrust{}},
		{name: "typed nil inspector", scope: NewScope("", "", testPaths{}), inspector: (*fakeHookInspector)(nil), trust: &fakeHookTrust{}},
		{name: "trust", scope: NewScope("", "", testPaths{}), inspector: &fakeHookInspector{}},
		{name: "typed nil trust", scope: NewScope("", "", testPaths{}), inspector: &fakeHookInspector{}, trust: (*fakeHookTrust)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if hooks, err := NewHooks(test.scope, test.inspector, test.trust, nil); err == nil || hooks != nil {
				t.Fatalf("NewHooks = (%v, %v), want incomplete construction rejected", hooks, err)
			}
		})
	}
}

func newHooks(t *testing.T, scope *Scope, inspector HookInspector, trust HookTrustStore, publish invalidation.Publish) *Hooks {
	t.Helper()
	if inspector == nil {
		inspector = &fakeHookInspector{}
	}
	if trust == nil {
		trust = &fakeHookTrust{}
	}
	hooks, err := NewHooks(scope, inspector, trust, publish)
	if err != nil {
		t.Fatal(err)
	}
	return hooks
}

func TestRuntimeInspectUsesInspectionPort(t *testing.T) {
	inspector := &fakeHookInspector{
		inspection: apphooks.Inspection{
			ProjectRoot:    "/repo",
			ProjectTrusted: true,
			Hooks: []hooks.Hook{{
				Event:   hooks.UserPromptSubmit,
				Command: "make test",
				Scope:   hooks.ScopeGlobal,
				Source:  "/home/.flame/hooks.json",
			}},
		},
	}
	c := newHooks(t, NewScope("", "", testPaths{}), inspector, nil, nil)

	got, err := c.Inspect(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if inspector.cwd != "/repo" {
		t.Fatalf("inspect cwd = %q, want /repo", inspector.cwd)
	}
	if got.ProjectRoot != "/repo" || !got.ProjectTrusted || len(got.Hooks) != 1 || got.Hooks[0].Hook.Command != "make test" || !got.Hooks[0].Active {
		t.Fatalf("Inspect = %+v", got)
	}
}

func TestRuntimeInspectRejectsInvalidInspection(t *testing.T) {
	c := newHooks(t, NewScope("", "", testPaths{}), &fakeHookInspector{inspection: apphooks.Inspection{
		ProjectRoot: "/other",
	}}, nil, nil)

	if _, err := c.Inspect(context.Background(), "/repo"); err == nil {
		t.Fatal("Inspect accepted an unrelated project root")
	}
}

type fakeHookInspector struct {
	cwd        string
	inspection apphooks.Inspection
	err        error
}

func (f *fakeHookInspector) Inspect(_ context.Context, cwd string) (apphooks.Inspection, error) {
	f.cwd = cwd
	return f.inspection, f.err
}

func TestRuntimeInspectPreservesInspectorFailure(t *testing.T) {
	wantErr := errors.New("hook trust unavailable")
	c := newHooks(t, NewScope("", "", testPaths{}), &fakeHookInspector{err: wantErr}, nil, nil)

	if _, err := c.Inspect(context.Background(), "/repo"); !errors.Is(err, wantErr) {
		t.Fatalf("Inspect error = %v, want %v", err, wantErr)
	}
}

func TestHookTrustPublishesOnlyCommittedChanges(t *testing.T) {
	trust := &fakeHookTrust{}
	var notices []invalidation.Notice
	hooks := newHooks(t,
		NewScope("", "", testPaths{}), nil, trust,
		func(notice invalidation.Notice) { notices = append(notices, notice) },
	)

	if err := hooks.SetProjectTrust(t.Context(), "/repo", true); err != nil {
		t.Fatal(err)
	}
	if trust.trusted != "/repo" || !reflect.DeepEqual(notices, []invalidation.Notice{{Resource: invalidation.Hooks}}) {
		t.Fatalf("trust=%q invalidations=%+v", trust.trusted, notices)
	}

	trust.err = errors.New("write failed")
	if err := hooks.SetProjectTrust(t.Context(), "/repo", false); !errors.Is(err, trust.err) {
		t.Fatalf("SetProjectTrust err = %v, want %v", err, trust.err)
	}
	if len(notices) != 1 {
		t.Fatalf("failed mutation published %+v", notices)
	}
}

type fakeHookTrust struct {
	trusted   string
	untrusted string
	err       error
}

func (f *fakeHookTrust) Trust(_ context.Context, root string) error {
	f.trusted = root
	return f.err
}

func (f *fakeHookTrust) Untrust(_ context.Context, root string) error {
	f.untrusted = root
	return f.err
}
