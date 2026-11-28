package workspace

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/samber/lo"
)

type Discovery struct {
	scope     *Scope
	sessions  sessionLister
	locations workspaceLocations
	agentDocs agentDocFinder
}

type sessionLister interface {
	List(ctx context.Context) ([]session.Session, error)
}

type workspaceLocations interface {
	directoryInspector
	Resolve(path string) (Info, error)
}

func NewDiscovery(scope *Scope, sessions sessionLister, locations workspaceLocations, agentDocs agentDocFinder) (*Discovery, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "scope", value: scope},
		{name: "session catalog", value: sessions},
		{name: "workspace locations", value: locations},
		{name: "agent document finder", value: agentDocs},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("workspace: discovery %s is required", required.name)
		}
	}
	return &Discovery{scope: scope, sessions: sessions, locations: locations, agentDocs: agentDocs}, nil
}

type Summary struct {
	Workspace    Info
	Name         string
	SessionCount int
	LastActiveAt time.Time
}

func (d *Discovery) Resolve(path string) (Info, error) {
	if path == "" {
		path = d.scope.defaultWorkspacePath
	}
	return d.locations.Resolve(path)
}

func (d *Discovery) Workspaces(ctx context.Context) ([]Summary, error) {
	sessions, err := d.sessions.List(ctx)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]*Summary)
	for _, value := range sessions {
		path := value.Workspace().Path()
		workspace := byPath[path]
		if workspace == nil {
			inspection, err := d.locations.Inspect(path)
			if err != nil {
				return nil, err
			}
			workspace = &Summary{
				Workspace: Info{Path: path, DirectoryInspection: inspection},
				Name:      filepath.Base(path),
			}
			byPath[path] = workspace
		}
		workspace.SessionCount++
		if value.UpdatedAt().After(workspace.LastActiveAt) {
			workspace.LastActiveAt = value.UpdatedAt()
		}
	}
	workspaces := make([]Summary, 0, len(byPath))
	for _, workspace := range byPath {
		workspaces = append(workspaces, *workspace)
	}
	slices.SortFunc(workspaces, compareWorkspaceSummaries)
	return workspaces, nil
}

func compareWorkspaceSummaries(a, b Summary) int {
	if order := b.LastActiveAt.Compare(a.LastActiveAt); order != 0 {
		return order
	}
	return cmp.Compare(a.Workspace.Path, b.Workspace.Path)
}

type AgentDoc struct {
	Path  string
	Scope AgentDocScope
}

type agentDocFinder interface {
	Find(ctx context.Context, cwd, home string) ([]AgentDocFile, error)
}

func (d *Discovery) AgentDocs(ctx context.Context, cwd string) ([]AgentDoc, error) {
	root, err := d.scope.ResolveRoot(cwd)
	if err != nil {
		return nil, err
	}
	files, err := d.agentDocs.Find(ctx, root, d.scope.userHome)
	if err != nil {
		return nil, err
	}
	if err := ValidateAgentDocumentCascade(files); err != nil {
		return nil, err
	}
	docs := make([]AgentDoc, 0, len(files))
	for _, file := range files {
		switch file.Scope {
		case AgentDocScopeHome, AgentDocScopeRepositoryRoot, AgentDocScopeCWD:
			docs = append(docs, AgentDoc{Path: file.Path, Scope: file.Scope})
		default:
			return nil, fmt.Errorf("workspace: unsupported agent document scope %q", file.Scope)
		}
	}
	return docs, nil
}
