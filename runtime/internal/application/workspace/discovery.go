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
	scope      *Scope
	workspaces catalog
	agentDocs  agentDocFinder
}

func NewDiscovery(scope *Scope, workspaces catalog, agentDocs agentDocFinder) (*Discovery, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "scope", value: scope},
		{name: "catalog", value: workspaces},
		{name: "agent document finder", value: agentDocs},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("workspace: discovery %s is required", required.name)
		}
	}
	return &Discovery{scope: scope, workspaces: workspaces, agentDocs: agentDocs}, nil
}

type Resolved struct {
	Path        string
	ProjectRoot string
	Missing     bool
}

type Summary struct {
	Name         string
	Path         string
	ProjectRoot  string
	Missing      bool
	SessionCount int
	LastActiveAt time.Time
}

type catalog interface {
	List(ctx context.Context) ([]session.Session, error)
	InspectWorkspace(cwd string) (Resolved, error)
}

func (d *Discovery) Resolve(path string) (Resolved, error) {
	if path == "" {
		path = d.scope.defaultWorkspacePath
	}
	return d.workspaces.InspectWorkspace(path)
}

func (d *Discovery) Workspaces(ctx context.Context) ([]Summary, error) {
	sessions, err := d.workspaces.List(ctx)
	if err != nil {
		return nil, err
	}
	workspaces := workspacesFromSessions(sessions)
	resolved := make([]Summary, 0, len(workspaces))
	byPath := make(map[string]int, len(workspaces))
	for _, workspace := range workspaces {
		identity, err := d.workspaces.InspectWorkspace(workspace.Path)
		if err != nil {
			return nil, err
		}
		workspace.Path = identity.Path
		workspace.ProjectRoot = identity.ProjectRoot
		workspace.Missing = identity.Missing
		workspace.Name = filepath.Base(identity.Path)
		if index, exists := byPath[identity.Path]; exists {
			resolved[index].SessionCount += workspace.SessionCount
			if workspace.LastActiveAt.After(resolved[index].LastActiveAt) {
				resolved[index].LastActiveAt = workspace.LastActiveAt
			}
			continue
		}
		byPath[identity.Path] = len(resolved)
		resolved = append(resolved, workspace)
	}
	slices.SortFunc(resolved, compareWorkspaceSummaries)
	return resolved, nil
}

func workspacesFromSessions(sessions []session.Session) []Summary {
	byPath := map[string]*Summary{}
	for _, sessionValue := range sessions {
		path := sessionValue.Workspace().Path()
		workspace := byPath[path]
		if workspace == nil {
			workspace = &Summary{Path: path, Name: filepath.Base(path)}
			byPath[path] = workspace
		}
		workspace.SessionCount++
		if workspace.LastActiveAt.IsZero() || sessionValue.UpdatedAt().After(workspace.LastActiveAt) {
			workspace.LastActiveAt = sessionValue.UpdatedAt()
		}
	}
	workspaces := make([]Summary, 0, len(byPath))
	for _, workspace := range byPath {
		workspaces = append(workspaces, *workspace)
	}
	slices.SortFunc(workspaces, compareWorkspaceSummaries)
	return workspaces
}

func compareWorkspaceSummaries(a, b Summary) int {
	if order := b.LastActiveAt.Compare(a.LastActiveAt); order != 0 {
		return order
	}
	return cmp.Compare(a.Path, b.Path)
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
		case AgentDocScopeHome, AgentDocScopeProjectRoot, AgentDocScopeCWD:
			docs = append(docs, AgentDoc{Path: file.Path, Scope: file.Scope})
		default:
			return nil, fmt.Errorf("workspace: unsupported agent document scope %q", file.Scope)
		}
	}
	return docs, nil
}
