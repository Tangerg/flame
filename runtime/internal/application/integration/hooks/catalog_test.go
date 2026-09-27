package hooks

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	domain "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
)

func TestNewCatalogRequiresCompleteDependencies(t *testing.T) {
	for _, test := range []struct {
		name      string
		roots     Roots
		inspector Inspector
		trust     TrustStore
	}{
		{name: "roots", inspector: &fakeInspector{}, trust: &fakeHookTrust{}},
		{name: "typed nil roots", roots: (*catalogRoots)(nil), inspector: &fakeInspector{}, trust: &fakeHookTrust{}},
		{name: "inspector", roots: catalogRoots{}, trust: &fakeHookTrust{}},
		{name: "typed nil inspector", roots: catalogRoots{}, inspector: (*fakeInspector)(nil), trust: &fakeHookTrust{}},
		{name: "trust", roots: catalogRoots{}, inspector: &fakeInspector{}},
		{name: "typed nil trust", roots: catalogRoots{}, inspector: &fakeInspector{}, trust: (*fakeHookTrust)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if hooks, err := NewCatalog(test.roots, test.inspector, test.trust, nil); err == nil || hooks != nil {
				t.Fatalf("NewCatalog = (%v, %v), want incomplete construction rejected", hooks, err)
			}
		})
	}
}

func newCatalog(t *testing.T, roots Roots, inspector Inspector, trust TrustStore, publish invalidation.Publish) *Catalog {
	t.Helper()
	if inspector == nil {
		inspector = &fakeInspector{}
	}
	if trust == nil {
		trust = &fakeHookTrust{}
	}
	hooks, err := NewCatalog(roots, inspector, trust, publish)
	if err != nil {
		t.Fatal(err)
	}
	return hooks
}

func TestCatalogInspectUsesInspectionPort(t *testing.T) {
	inspector := &fakeInspector{
		inspection: Inspection{
			ProjectRoot:    "/repo",
			ProjectTrusted: true,
			Hooks: []domain.Hook{{
				Event:   domain.UserPromptSubmit,
				Command: "make test",
				Scope:   domain.ScopeGlobal,
				Source:  "/home/.flame/hooks.json",
			}},
		},
	}
	c := newCatalog(t, catalogRoots{}, inspector, nil, nil)

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

func TestCatalogInspectRejectsInvalidInspection(t *testing.T) {
	c := newCatalog(t, catalogRoots{}, &fakeInspector{inspection: Inspection{
		ProjectRoot: "/other",
	}}, nil, nil)

	if _, err := c.Inspect(context.Background(), "/repo"); err == nil {
		t.Fatal("Inspect accepted an unrelated project root")
	}
}

func TestCatalogResolvesWorkspaceBeforeReadingOrChangingTrust(t *testing.T) {
	wantErr := errors.New("workspace unavailable")
	inspector := &fakeInspector{}
	trust := &fakeHookTrust{}
	catalog := newCatalog(t, catalogRoots{err: wantErr}, inspector, trust, nil)
	if _, err := catalog.Inspect(t.Context(), "/missing"); !errors.Is(err, wantErr) {
		t.Fatalf("Inspect = %v, want root resolution failure", err)
	}
	if err := catalog.SetProjectTrust(t.Context(), "/missing", true); !errors.Is(err, wantErr) {
		t.Fatalf("SetProjectTrust = %v, want root resolution failure", err)
	}
	if inspector.cwd != "" || trust.trusted != "" || trust.untrusted != "" {
		t.Fatal("failed root resolution reached the hook inspector or trust store")
	}

	catalog = newCatalog(t, catalogRoots{root: "/repo"}, inspector, trust, nil)
	if _, err := catalog.Inspect(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SetProjectTrust(t.Context(), "/repo/alias", false); err != nil {
		t.Fatal(err)
	}
	if inspector.cwd != "/repo" || trust.untrusted != "/repo" {
		t.Fatalf("resolved workspace not used: inspected=%q untrusted=%q", inspector.cwd, trust.untrusted)
	}
}

func TestCatalogActivatesGlobalHooksIndependentlyOfProjectTrust(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		inspection := validInspection()
		inspection.ProjectTrusted = trusted
		catalog := newCatalog(t, catalogRoots{}, &fakeInspector{inspection: inspection}, nil, nil)
		view, err := catalog.Inspect(t.Context(), "/repo/pkg")
		if err != nil {
			t.Fatal(err)
		}
		if len(view.Hooks) != 2 || !view.Hooks[0].Active || view.Hooks[1].Active != trusted {
			t.Fatalf("trusted=%v resolved hooks=%+v", trusted, view.Hooks)
		}
	}
}

type fakeInspector struct {
	cwd        string
	inspection Inspection
	err        error
}

func (f *fakeInspector) Inspect(_ context.Context, cwd string) (Inspection, error) {
	f.cwd = cwd
	return f.inspection, f.err
}

func TestCatalogInspectPreservesInspectorFailure(t *testing.T) {
	wantErr := errors.New("hook trust unavailable")
	c := newCatalog(t, catalogRoots{}, &fakeInspector{err: wantErr}, nil, nil)

	if _, err := c.Inspect(context.Background(), "/repo"); !errors.Is(err, wantErr) {
		t.Fatalf("Inspect error = %v, want %v", err, wantErr)
	}
}

func TestCatalogTrustPublishesOnlyCommittedChanges(t *testing.T) {
	trust := &fakeHookTrust{}
	var notices []invalidation.Notice
	hooks := newCatalog(t,
		catalogRoots{}, nil, trust,
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

type catalogRoots struct {
	root string
	err  error
}

func (r catalogRoots) ResolveRoot(cwd string) (string, error) {
	if r.root != "" || r.err != nil {
		return r.root, r.err
	}
	return cwd, nil
}
