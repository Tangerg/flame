package workspace

import (
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
)

// AuthoredResource is the closed set of file-backed product resources whose
// query projections can be changed by another process.
type AuthoredResource string

const (
	AuthoredHooks  AuthoredResource = AuthoredResource(invalidation.Hooks)
	AuthoredSkills AuthoredResource = AuthoredResource(invalidation.Skills)
)

// Valid reports whether a is one externally authored product source.
func (a AuthoredResource) Valid() bool {
	return a == AuthoredHooks || a == AuthoredSkills
}

// InvalidationResource maps a to the same application-owned change
// vocabulary. Invalid resources map to the invalid zero value.
func (a AuthoredResource) InvalidationResource() invalidation.Resource {
	if !a.Valid() {
		return ""
	}
	return invalidation.Resource(a)
}

// AuthoredScope is one canonical workspace identity and its project root.
// Filesystem layout stays outside Application; these are the semantic roots
// already used by the Hooks and Skills use cases.
type AuthoredScope struct {
	Workspace   string
	ProjectRoot string
}

// AuthoredResourceWatcher adapts external filesystem state into semantic
// resource changes. Implementations own filenames, cascades, notification
// mechanisms, and symlink identity. Watch borrows scopes and resources for the
// call; implementations own any retained observation configuration. report
// receives the first failure of each background outage; it is the only place
// an outage is observable, so the caller decides what it ends.
type AuthoredResourceWatcher interface {
	Watch(
		scopes []AuthoredScope,
		resources []AuthoredResource,
		notify func(AuthoredResource),
		report func(error),
	) (AuthoredObservation, error)
}

// AuthoredChange identifies exact file-backed resource members that were
// changed through an authoritative use case.
type AuthoredChange struct {
	Resource   AuthoredResource
	Identities []string
}

// AuthoredObservation owns one live semantic resource observation. Accept
// records exact members after another authoritative path announced the change.
// Accept borrows changes for the call and does not mutate them.
type AuthoredObservation interface {
	io.Closer
	Accept(changes []AuthoredChange) error
}

// IdentityInspector supplies the one live identity fact needed to distinguish
// a nested workspace root from its project-discovery root.
type IdentityInspector interface {
	Inspect(path string) (Resolved, error)
}

// AuthoredWatch resolves client workspace identities before delegating the
// external observation mechanism. It does not know transport topics.
type AuthoredWatch struct {
	scope      *Scope
	workspaces IdentityInspector
	watcher    AuthoredResourceWatcher
	mu         sync.Mutex
	active     map[*managedAuthoredObservation]struct{}
}

func NewAuthoredWatch(scope *Scope, workspaces IdentityInspector, watcher AuthoredResourceWatcher) (*AuthoredWatch, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "scope", value: scope},
		{name: "workspace inspector", value: workspaces},
		{name: "resource watcher", value: watcher},
	} {
		if dependency.Missing(required.value) {
			return nil, fmt.Errorf("workspace: authored watch %s is required", required.name)
		}
	}
	return &AuthoredWatch{
		scope: scope, workspaces: workspaces, watcher: watcher,
		active: make(map[*managedAuthoredObservation]struct{}),
	}, nil
}

// Watch starts one caller-owned observation. An empty cwd list still observes
// the implementation's global resource scopes. cwds and resources are borrowed
// for the call.
func (a *AuthoredWatch) Watch(
	cwds []string,
	resources []AuthoredResource,
	notify func(AuthoredResource),
	report func(error),
) (AuthoredObservation, error) {
	resources, err := distinctAuthoredResources(resources)
	if err != nil {
		return nil, err
	}
	if len(resources) == 0 {
		return nopAuthoredWatch{}, nil
	}
	type authoredScopeKey struct {
		workspace   string
		projectRoot string
	}
	seen := make(map[authoredScopeKey]struct{}, len(cwds))
	scopes := make([]AuthoredScope, 0, len(cwds))
	for _, cwd := range cwds {
		root, err := a.scope.ResolveRoot(cwd)
		if err != nil {
			return nil, err
		}
		resolved, err := a.workspaces.Inspect(root)
		if err != nil {
			return nil, err
		}
		if resolved.Missing || resolved.ProjectRoot == "" {
			return nil, ErrCWDUnavailable
		}
		identity := authoredScopeKey{workspace: root, projectRoot: resolved.ProjectRoot}
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		scopes = append(scopes, AuthoredScope{Workspace: root, ProjectRoot: resolved.ProjectRoot})
	}
	inner, err := a.watcher.Watch(scopes, resources, notify, report)
	if err != nil {
		return nil, err
	}
	managed := &managedAuthoredObservation{owner: a, inner: inner}
	a.mu.Lock()
	a.active[managed] = struct{}{}
	a.mu.Unlock()
	return managed, nil
}

// Accept records exact members changed by a workspace use case before its
// invalidation is published. Failure is intentionally best-effort: the durable
// mutation already committed, and a later callback is a safe duplicate rather
// than grounds to report that commit as failed.
func (a *AuthoredWatch) Accept(change AuthoredChange) {
	if a == nil || len(change.Identities) == 0 {
		return
	}
	a.mu.Lock()
	active := make([]*managedAuthoredObservation, 0, len(a.active))
	for observation := range a.active {
		active = append(active, observation)
	}
	a.mu.Unlock()
	for _, observation := range active {
		if err := observation.inner.Accept([]AuthoredChange{change}); err != nil {
			slog.Warn("workspace: accept authored resource change", "resource", change.Resource, "error", err)
		}
	}
}

type managedAuthoredObservation struct {
	owner    *AuthoredWatch
	inner    AuthoredObservation
	once     sync.Once
	closeErr error
}

func (m *managedAuthoredObservation) Accept(changes []AuthoredChange) error {
	return m.inner.Accept(changes)
}

func (m *managedAuthoredObservation) Close() error {
	m.once.Do(func() {
		m.owner.mu.Lock()
		delete(m.owner.active, m)
		m.owner.mu.Unlock()
		m.closeErr = m.inner.Close()
	})
	return m.closeErr
}

// distinctAuthoredResources refuses a resource outside the closed set rather
// than dropping it: a request that named only unknown resources would
// otherwise start an observation that can never fire.
func distinctAuthoredResources(resources []AuthoredResource) ([]AuthoredResource, error) {
	out := make([]AuthoredResource, 0, len(resources))
	for _, resource := range resources {
		if !resource.Valid() {
			return nil, fmt.Errorf("workspace: authored resource %q is not observable", resource)
		}
		if !slices.Contains(out, resource) {
			out = append(out, resource)
		}
	}
	return out, nil
}

type nopAuthoredWatch struct{}

func (nopAuthoredWatch) Close() error                  { return nil }
func (nopAuthoredWatch) Accept([]AuthoredChange) error { return nil }
