package workspace

import (
	"errors"
	"testing"
)

func TestNewScopeRequiresPathResolver(t *testing.T) {
	for _, paths := range []Paths{nil, (*testPaths)(nil)} {
		if scope, err := NewScope("", "", paths); err == nil || scope != nil {
			t.Fatalf("NewScope = (%v, %v), want missing resolver rejected", scope, err)
		}
	}
}

func newScope(t *testing.T, defaultWorkspacePath, userHome string, paths Paths) *Scope {
	t.Helper()
	scope, err := NewScope(defaultWorkspacePath, userHome, paths)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func TestScopeResolvesSelectedRootAndPreservesFailure(t *testing.T) {
	paths := &scopePaths{}
	scope := newScope(t, "/default", "/home", paths)
	for _, test := range []struct{ requested, selected string }{
		{selected: "/default"},
		{requested: "/explicit", selected: "/explicit"},
	} {
		resolved, err := scope.ResolveRoot(test.requested)
		if err != nil || resolved != test.selected || paths.selected != test.selected {
			t.Fatalf("ResolveRoot(%q) = (%q, %v), selected %q", test.requested, resolved, err, paths.selected)
		}
	}
	paths.err = errors.New("filesystem unavailable")
	if _, err := scope.ResolveRoot(""); !errors.Is(err, ErrCWDUnavailable) || !errors.Is(err, paths.err) {
		t.Fatalf("ResolveRoot error = %v, want workspace category and filesystem cause", err)
	}
}

type scopePaths struct {
	testPaths
	selected string
	err      error
}

func (p *scopePaths) ResolveExistingDir(path string) (string, error) {
	p.selected = path
	return path, p.err
}
