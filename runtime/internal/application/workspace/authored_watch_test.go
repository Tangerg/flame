package workspace

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

type recordingAuthoredWatcher struct {
	scopes    []AuthoredScope
	resources []AuthoredResource
	accepted  []AuthoredChange
}

func (r *recordingAuthoredWatcher) Watch(
	scopes []AuthoredScope,
	resources []AuthoredResource,
	_ func(AuthoredResource),
) (AuthoredObservation, error) {
	r.scopes = slices.Clone(scopes)
	r.resources = slices.Clone(resources)
	return recordingAuthoredObservation{owner: r}, nil
}

type recordingAuthoredObservation struct{ owner *recordingAuthoredWatcher }

func (r recordingAuthoredObservation) Close() error { return nil }
func (r recordingAuthoredObservation) Accept(changes []AuthoredChange) error {
	for _, change := range changes {
		change.Identities = slices.Clone(change.Identities)
		r.owner.accepted = append(r.owner.accepted, change)
	}
	return nil
}

func TestAuthoredWatchResolvesAndDeduplicatesScopes(t *testing.T) {
	root := t.TempDir()
	watcher := &recordingAuthoredWatcher{}
	useCases := newAuthoredWatch(t, newScope(t, root, root, testPaths{}), staticWorkspaceInspector{
		resolved: Resolved{Path: root, ProjectRoot: root},
	}, watcher)
	closer, err := useCases.Watch(
		[]string{"", root},
		[]AuthoredResource{AuthoredKnowledge, AuthoredKnowledge, AuthoredHooks, AuthoredSkills},
		func(AuthoredResource) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closer.Close() }()
	if !reflect.DeepEqual(watcher.scopes, []AuthoredScope{{Workspace: root, ProjectRoot: root}}) {
		t.Fatalf("scopes = %+v", watcher.scopes)
	}
	if !reflect.DeepEqual(watcher.resources, []AuthoredResource{AuthoredKnowledge, AuthoredHooks, AuthoredSkills}) {
		t.Fatalf("resources = %+v", watcher.resources)
	}
}

func TestAuthoredWatchOwnsObservationScopes(t *testing.T) {
	root := t.TempDir()
	cwds := []string{filepath.Join(root, "first"), filepath.Join(root, "second")}
	watcher := &recordingAuthoredWatcher{}
	inspector := staticWorkspaceInspector{
		resolved: Resolved{Path: root, ProjectRoot: root},
	}
	useCases := newAuthoredWatch(t, newScope(t, root, root, testPaths{}), inspector, watcher)

	observation, err := useCases.Watch(cwds, []AuthoredResource{AuthoredSkills}, func(AuthoredResource) {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observation.Close() }()
	cwds[1] = filepath.Join(root, "changed")
	if got := watcher.scopes[1].Workspace; got != filepath.Join(root, "second") {
		t.Fatalf("second scope after caller reused input = %q", got)
	}
}

func TestAuthoredWatchOwnsChangesForEveryObservation(t *testing.T) {
	root := t.TempDir()
	watcher := &recordingAuthoredWatcher{}
	useCases := newAuthoredWatch(t, newScope(t, root, root, testPaths{}), staticWorkspaceInspector{
		resolved: Resolved{Path: root, ProjectRoot: root},
	}, watcher)
	first, err := useCases.Watch(nil, []AuthoredResource{AuthoredSkills}, func(AuthoredResource) {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := useCases.Watch(nil, []AuthoredResource{AuthoredSkills}, func(AuthoredResource) {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()

	direct := []AuthoredChange{{Resource: AuthoredSkills, Identities: []string{"direct"}}}
	if err := first.Accept(direct); err != nil {
		t.Fatal(err)
	}
	if direct[0].Identities[0] != "direct" {
		t.Fatalf("direct caller change mutated to %q", direct[0].Identities[0])
	}
	direct[0].Identities[0] = "reused direct"

	broadcast := AuthoredChange{Resource: AuthoredSkills, Identities: []string{"broadcast"}}
	useCases.Accept(broadcast)
	if broadcast.Identities[0] != "broadcast" {
		t.Fatalf("broadcast caller change mutated to %q", broadcast.Identities[0])
	}
	broadcast.Identities[0] = "reused broadcast"
	want := []AuthoredChange{
		{Resource: AuthoredSkills, Identities: []string{"direct"}},
		{Resource: AuthoredSkills, Identities: []string{"broadcast"}},
		{Resource: AuthoredSkills, Identities: []string{"broadcast"}},
	}
	if !reflect.DeepEqual(watcher.accepted, want) {
		t.Fatalf("observation changes = %v, want %v", watcher.accepted, want)
	}
}

func TestNewAuthoredWatchRequiresCompleteDependencies(t *testing.T) {
	scope := newScope(t, "", "", testPaths{})
	for _, test := range []struct {
		name      string
		scope     *Scope
		inspector KnowledgeWorkspaceInspector
		watcher   AuthoredResourceWatcher
	}{
		{name: "scope", inspector: staticWorkspaceInspector{}, watcher: &recordingAuthoredWatcher{}},
		{name: "inspector", scope: scope, watcher: &recordingAuthoredWatcher{}},
		{name: "typed nil inspector", scope: scope, inspector: (*staticWorkspaceInspector)(nil), watcher: &recordingAuthoredWatcher{}},
		{name: "watcher", scope: scope, inspector: staticWorkspaceInspector{}},
		{name: "typed nil watcher", scope: scope, inspector: staticWorkspaceInspector{}, watcher: (*recordingAuthoredWatcher)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if watch, err := NewAuthoredWatch(test.scope, test.inspector, test.watcher); err == nil || watch != nil {
				t.Fatalf("NewAuthoredWatch = (%v, %v), want incomplete construction rejected", watch, err)
			}
		})
	}
}

func newAuthoredWatch(t *testing.T, scope *Scope, inspector KnowledgeWorkspaceInspector, watcher AuthoredResourceWatcher) *AuthoredWatch {
	t.Helper()
	watch, err := NewAuthoredWatch(scope, inspector, watcher)
	if err != nil {
		t.Fatal(err)
	}
	return watch
}

func TestAuthoredWatchRejectsInvalidWorkspaceInspection(t *testing.T) {
	root := t.TempDir()
	watcher := &recordingAuthoredWatcher{}
	useCases := newAuthoredWatch(t, newScope(t, root, root, testPaths{}), staticWorkspaceInspector{
		resolved: Resolved{Path: root, ProjectRoot: filepath.Join(root, "nested")},
	}, watcher)
	if _, err := useCases.Watch([]string{root}, []AuthoredResource{AuthoredSkills}, func(AuthoredResource) {}); err == nil {
		t.Fatal("Watch accepted invalid workspace inspection")
	}
	if watcher.scopes != nil {
		t.Fatal("invalid workspace inspection reached watcher")
	}
}

type staticWorkspaceInspector struct{ resolved Resolved }

func (s staticWorkspaceInspector) Inspect(string) (Resolved, error) { return s.resolved, nil }
