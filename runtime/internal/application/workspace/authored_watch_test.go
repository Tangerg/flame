package workspace

import (
	"path/filepath"
	"reflect"
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
	r.scopes = scopes
	r.resources = resources
	return recordingAuthoredObservation{owner: r}, nil
}

type recordingAuthoredObservation struct{ owner *recordingAuthoredWatcher }

func (r recordingAuthoredObservation) Close() error { return nil }
func (r recordingAuthoredObservation) Accept(changes []AuthoredChange) error {
	r.owner.accepted = append(r.owner.accepted, changes...)
	return nil
}

type callbackWorkspaceInspector struct {
	projectRoot string
	onFirst     func()
	inspections int
}

func (c *callbackWorkspaceInspector) Inspect(path string) (Resolved, error) {
	c.inspections++
	if c.inspections == 1 && c.onFirst != nil {
		c.onFirst()
	}
	return Resolved{Path: path, ProjectRoot: c.projectRoot}, nil
}

type mutatingAuthoredWatcher struct{ identities []string }

func (m *mutatingAuthoredWatcher) Watch(
	[]AuthoredScope,
	[]AuthoredResource,
	func(AuthoredResource),
) (AuthoredObservation, error) {
	return mutatingAuthoredObservation{owner: m}, nil
}

type mutatingAuthoredObservation struct{ owner *mutatingAuthoredWatcher }

func (mutatingAuthoredObservation) Close() error { return nil }
func (m mutatingAuthoredObservation) Accept(changes []AuthoredChange) error {
	m.owner.identities = append(m.owner.identities, changes[0].Identities[0])
	changes[0].Identities[0] = "changed"
	return nil
}

func TestAuthoredWatchResolvesAndDeduplicatesScopes(t *testing.T) {
	root := t.TempDir()
	watcher := &recordingAuthoredWatcher{}
	useCases := NewAuthoredWatch(NewScope(root, root, testPaths{}), staticWorkspaceInspector{
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

func TestAuthoredWatchOwnsCWDsBeforeWorkspaceInspection(t *testing.T) {
	root := t.TempDir()
	cwds := []string{filepath.Join(root, "first"), filepath.Join(root, "second")}
	watcher := &recordingAuthoredWatcher{}
	inspector := &callbackWorkspaceInspector{
		projectRoot: root,
		onFirst:     func() { cwds[1] = filepath.Join(root, "changed") },
	}
	useCases := NewAuthoredWatch(NewScope(root, root, testPaths{}), inspector, watcher)

	observation, err := useCases.Watch(cwds, []AuthoredResource{AuthoredSkills}, func(AuthoredResource) {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observation.Close() }()
	if got := watcher.scopes[1].Workspace; got != filepath.Join(root, "second") {
		t.Fatalf("second scope after inspector changed caller input = %q", got)
	}
}

func TestAuthoredWatchOwnsChangesForEveryObservation(t *testing.T) {
	root := t.TempDir()
	watcher := &mutatingAuthoredWatcher{}
	useCases := NewAuthoredWatch(NewScope(root, root, testPaths{}), staticWorkspaceInspector{
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

	broadcast := AuthoredChange{Resource: AuthoredSkills, Identities: []string{"broadcast"}}
	useCases.Accept(broadcast)
	if broadcast.Identities[0] != "broadcast" {
		t.Fatalf("broadcast caller change mutated to %q", broadcast.Identities[0])
	}
	if !reflect.DeepEqual(watcher.identities, []string{"direct", "broadcast", "broadcast"}) {
		t.Fatalf("observation identities = %v", watcher.identities)
	}
}

func TestAuthoredWatchRejectsInvalidWorkspaceInspection(t *testing.T) {
	root := t.TempDir()
	watcher := &recordingAuthoredWatcher{}
	useCases := NewAuthoredWatch(NewScope(root, root, testPaths{}), staticWorkspaceInspector{
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
