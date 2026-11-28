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

type AuthoredObservation interface {
	io.Closer
	ReconcileSkillPublication(paths []string) error
}

type AuthoredWatch struct {
	scope         *Scope
	directories   directoryInspector
	watcher       AuthoredResourceWatcher
	invalidations invalidation.Publish
	mu            sync.Mutex
	skillWatchers map[*managedAuthoredObservation]struct{}
}

func NewAuthoredWatch(scope *Scope, directories directoryInspector, watcher AuthoredResourceWatcher, invalidations invalidation.Publish) (*AuthoredWatch, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "scope", value: scope},
		{name: "workspace inspector", value: directories},
		{name: "resource watcher", value: watcher},
		{name: "invalidations", value: invalidations},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("workspace: authored watch %s is required", required.name)
		}
	}
	return &AuthoredWatch{
		scope: scope, directories: directories, watcher: watcher, invalidations: invalidations,
		skillWatchers: make(map[*managedAuthoredObservation]struct{}),
	}, nil
}

func (a *AuthoredWatch) Watch(
	cwds []string,
	resources []AuthoredResource,
	notify func(AuthoredResource),
	report func(error),
) (io.Closer, error) {
	resources, err := distinctAuthoredResources(resources)
	if err != nil {
		return nil, err
	}
	if len(resources) == 0 {
		return nil, nil
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
		inspection, err := a.directories.Inspect(root)
		if err != nil {
			return nil, err
		}
		if inspection.Missing || inspection.RepositoryRoot == "" {
			return nil, ErrCWDUnavailable
		}
		identity := authoredScopeKey{workspace: root, repositoryRoot: inspection.RepositoryRoot}
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		scopes = append(scopes, AuthoredScope{Workspace: root, RepositoryRoot: inspection.RepositoryRoot})
	}
	inner, err := a.watcher.Watch(scopes, resources, notify, report)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(resources, AuthoredSkills) {
		return inner, nil
	}
	managed := &managedAuthoredObservation{owner: a, inner: inner}
	a.mu.Lock()
	a.skillWatchers[managed] = struct{}{}
	a.mu.Unlock()
	return managed, nil
}

func (a *AuthoredWatch) PublishSkillChanges(paths []string) {
	if len(paths) == 0 {
		return
	}
	a.mu.Lock()
	active := make([]*managedAuthoredObservation, 0, len(a.skillWatchers))
	for observation := range a.skillWatchers {
		active = append(active, observation)
	}
	a.mu.Unlock()
	for _, observation := range active {
		if err := observation.inner.ReconcileSkillPublication(paths); err != nil {
			slog.Warn("workspace: reconcile published Skills", telemetry.Error(err))
		}
	}
	a.invalidations.Notify(invalidation.Notice{Resource: invalidation.Skills})
}

type managedAuthoredObservation struct {
	owner    *AuthoredWatch
	inner    AuthoredObservation
	once     sync.Once
	closeErr error
}

func (m *managedAuthoredObservation) Close() error {
	m.once.Do(func() {
		m.owner.mu.Lock()
		delete(m.owner.skillWatchers, m)
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
