package workspace

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// RecipeLister discovers the precedence-resolved recipes visible from a working
// directory. Application validates visible-name identity and owns public order.
type RecipeLister interface {
	List(ctx context.Context, cwd string) ([]Recipe, error)
}

// Discovery owns workspace, recipe, and instruction-document discovery.
type Discovery struct {
	scope      *Scope
	workspaces Catalog
	agentDocs  AgentDocFinder
	recipes    RecipeLister
}

func NewDiscovery(scope *Scope, workspaces Catalog, agentDocs AgentDocFinder, recipes RecipeLister) *Discovery {
	return &Discovery{scope: scope, workspaces: workspaces, agentDocs: agentDocs, recipes: recipes}
}

// Recipes enumerates the one precedence-resolved Recipe per visible name,
// ordered by name.
func (d *Discovery) Recipes(ctx context.Context, cwd string) ([]Recipe, error) {
	root, err := d.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	if d.recipes == nil {
		return nil, nil
	}
	recipes, err := d.recipes.List(ctx, root)
	if err != nil {
		return nil, err
	}
	recipes = slices.Clone(recipes)
	if err := ValidateRecipeCascade(recipes); err != nil {
		return nil, err
	}
	slices.SortFunc(recipes, func(first, second Recipe) int {
		return cmp.Compare(first.Name, second.Name)
	})
	return recipes, nil
}

// Resolved is the current filesystem identity of one workspace ref.
type Resolved struct {
	Path        string
	ProjectRoot string
	Missing     bool
}

// Validate checks one complete live workspace identity. Missing describes
// availability only; the canonical path and its containing project root remain
// stable identities even after the directory disappears.
func (r Resolved) Validate() error {
	for _, candidate := range []struct {
		label string
		path  string
	}{
		{label: "path", path: r.Path},
		{label: "project root", path: r.ProjectRoot},
	} {
		label, path := candidate.label, candidate.path
		if path == "" || !utf8.ValidString(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return fmt.Errorf("workspace: resolved %s %q is not a canonical absolute path", label, path)
		}
	}
	relative, err := filepath.Rel(r.ProjectRoot, r.Path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("workspace: resolved path %q is outside project root %q", r.Path, r.ProjectRoot)
	}
	return nil
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

// Validate checks one complete user-facing workspace summary.
func (s Summary) Validate() error {
	if err := (Resolved{Path: s.Path, ProjectRoot: s.ProjectRoot, Missing: s.Missing}).Validate(); err != nil {
		return err
	}
	if s.Name != filepath.Base(s.Path) || !utf8.ValidString(s.Name) {
		return fmt.Errorf("workspace: summary name %q does not match path %q", s.Name, s.Path)
	}
	if s.SessionCount <= 0 {
		return fmt.Errorf("workspace: summary %q has invalid Session count %d", s.Path, s.SessionCount)
	}
	if s.LastActiveAt.IsZero() {
		return fmt.Errorf("workspace: summary %q has no activity time", s.Path)
	}
	return nil
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
	if d.workspaces == nil {
		return Resolved{}, errors.New("workspace: workspace catalog is not configured")
	}
	if path == "" {
		path = d.scope.defaultWorkspacePath
	}
	identity, err := d.workspaces.InspectWorkspace(path)
	if err != nil {
		return Resolved{}, err
	}
	if err := identity.Validate(); err != nil {
		return Resolved{}, err
	}
	return identity, nil
}

// Workspaces returns each non-empty session workspace once, newest-active first
// with canonical path ascending as the stable tie-breaker.
func (d *Discovery) Workspaces(ctx context.Context) ([]Summary, error) {
	if d.workspaces == nil {
		return nil, errors.New("workspace: workspace catalog is not configured")
	}
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
		if err := identity.Validate(); err != nil {
			return nil, err
		}
		workspace.Path = identity.Path
		workspace.ProjectRoot = identity.ProjectRoot
		workspace.Missing = identity.Missing
		workspace.Name = filepath.Base(identity.Path)
		if index, exists := byPath[identity.Path]; exists {
			if resolved[index].ProjectRoot != workspace.ProjectRoot || resolved[index].Missing != workspace.Missing {
				return nil, fmt.Errorf("workspace: aliases for %q disagree on live identity", identity.Path)
			}
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
	for index, workspace := range resolved {
		if err := workspace.Validate(); err != nil {
			return nil, fmt.Errorf("workspace: catalog row %d: %w", index+1, err)
		}
	}
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
	root, err := d.scope.root(cwd)
	if err != nil {
		return nil, err
	}
	if d.agentDocs == nil {
		return nil, errors.New("workspace: agent document finder is not configured")
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
