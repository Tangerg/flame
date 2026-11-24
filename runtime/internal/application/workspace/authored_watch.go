package workspace

import (
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"github.com/samber/lo"
)

type AuthoredResource string

const (
	AuthoredHooks  AuthoredResource = AuthoredResource(invalidation.Hooks)
	AuthoredSkills AuthoredResource = AuthoredResource(invalidation.Skills)
)

func (a AuthoredResource) Valid() bool {
	return a == AuthoredHooks || a == AuthoredSkills
}

func (a AuthoredResource) InvalidationResource() invalidation.Resource {
	if !a.Valid() {
		return ""
	}
	return invalidation.Resource(a)
}

type AuthoredScope struct {
	Workspace      string
	RepositoryRoot string
}

type AuthoredResourceWatcher interface {
	Watch(
		scopes []AuthoredScope,
		resources []AuthoredResource,
		notify func(AuthoredResource),
		report func(error),
	) (AuthoredObservation, error)
}

type AuthoredChange struct {
	Resource   AuthoredResource
	Identities []string
}

type AuthoredObservation interface {
	io.Closer
	Accept(changes []AuthoredChange) error
}

type identityInspector interface {
	Inspect(path string) (Resolved, error)
}

type AuthoredWatch struct {
	scope      *Scope
	workspaces identityInspector
	watcher    AuthoredResourceWatcher
	mu         sync.Mutex
	active     map[*managedAuthoredObservation]struct{}
}

func NewAuthoredWatch(scope *Scope, workspaces identityInspector, watcher AuthoredResourceWatcher) (*AuthoredWatch, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "scope", value: scope},
		{name: "workspace inspector", value: workspaces},
		{name: "resource watcher", value: watcher},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("workspace: authored watch %s is required", required.name)
		}
	}
	return &AuthoredWatch{
		scope: scope, workspaces: workspaces, watcher: watcher,
		active: make(map[*managedAuthoredObservation]struct{}),
	}, nil
}

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
		workspace      string
		repositoryRoot string
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
		if resolved.Missing || resolved.RepositoryRoot == "" {
			return nil, ErrCWDUnavailable
		}
		identity := authoredScopeKey{workspace: root, repositoryRoot: resolved.RepositoryRoot}
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		scopes = append(scopes, AuthoredScope{Workspace: root, RepositoryRoot: resolved.RepositoryRoot})
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
			slog.Warn("workspace: accept authored resource change", slog.Any("resource", change.Resource), telemetry.Error(err))
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
