package workspace

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// Discovery owns workspace and instruction-document discovery.
type Discovery struct {
	scope      *Scope
	workspaces Catalog
	agentDocs  AgentDocFinder
}

func NewDiscovery(scope *Scope, workspaces Catalog, agentDocs AgentDocFinder) (*Discovery, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "scope", value: scope},
		{name: "catalog", value: workspaces},
		{name: "agent document finder", value: agentDocs},
	} {
		if dependency.Missing(required.value) {
			return nil, fmt.Errorf("workspace: discovery %s is required", required.name)
		}
	}
	return &Discovery{scope: scope, workspaces: workspaces, agentDocs: agentDocs}, nil
}

// Resolved is the current filesystem identity of one workspace ref.
type Resolved struct {
	Path        string
	ProjectRoot string
	Missing     bool
}

// Summary is a distinct workspace identity derived from user-facing sessions.
type Summary struct {
	Name         string
	Path         string
	ProjectRoot  string
	Missing      bool
	SessionCount int
	LastActiveAt time.Time
}

// Catalog supplies the user-facing sessions and their current workspace
// identities. The session coordinator is the production implementation.
type Catalog interface {
	List(ctx context.Context) ([]session.Session, error)
	InspectWorkspace(cwd string) (Resolved, error)
}

// Resolve returns the canonical live workspace identity for path, using the
// host-provided default when path is empty.
func (d *Discovery) Resolve(path string) (Resolved, error) {
	if path == "" {
		path = d.scope.defaultWorkspacePath
	}
	return d.workspaces.InspectWorkspace(path)
}

// Workspaces returns each non-empty session workspace once, newest-active first
// with canonical path ascending as the stable tie-breaker.
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

// AgentDoc is one discovered instruction document with its cascade scope.
type AgentDoc struct {
	Path  string
	Scope AgentDocScope
}

// AgentDocFinder discovers the workspace instruction-document cascade in render
// order. Application validates unique source identity and phase order.
type AgentDocFinder interface {
	Find(ctx context.Context, cwd, home string) ([]AgentDocFile, error)
}

// AgentDocs returns the unique instruction-document cascade for one workspace in
// home, project-root, and cwd render phases.
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
