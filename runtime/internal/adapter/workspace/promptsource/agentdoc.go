package promptsource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/repository"
)

const agentDocName = "AGENTS.md"

type AgentDocs struct{}

func (AgentDocs) Find(ctx context.Context, cwd, home string) ([]workspaceapp.AgentDocFile, error) {
	if err := validateAgentDocPaths(cwd, home); err != nil {
		return nil, err
	}
	cwd = filepath.Clean(cwd)
	if home != "" {
		home = filepath.Clean(home)
	}

	d := &agentDocScan{seen: make(map[string]struct{})}
	if err := d.discoverHome(ctx, home); err != nil {
		return nil, err
	}
	if err := d.discoverRepositoryTree(ctx, cwd); err != nil {
		return nil, err
	}
	return d.files, nil
}

func validateAgentDocPaths(cwd, home string) error {
	if cwd == "" {
		return errors.New("promptsource: cwd is required")
	}
	if !filepath.IsAbs(cwd) {
		return errors.New("promptsource: cwd must be absolute")
	}
	if home != "" && !filepath.IsAbs(home) {
		return errors.New("promptsource: home must be absolute")
	}
	return nil
}

type agentDocScan struct {
	seen     map[string]struct{}
	files    []workspaceapp.AgentDocFile
	rawBytes int
}

func (a *agentDocScan) discoverHome(ctx context.Context, home string) (err error) {
	if home == "" {
		return nil
	}
	root, err := openDocumentRoot(home)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	return a.discoverIn(ctx, root, home, filepath.Join(home, ".agents"), workspaceapp.AgentDocScopeHome)
}

func (a *agentDocScan) discoverRepositoryTree(ctx context.Context, cwd string) (err error) {
	root, err := repository.Root(cwd)
	if err != nil {
		return fmt.Errorf("promptsource: locate repository root for %q: %w", cwd, err)
	}
	boundary, err := openDocumentRoot(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, boundary.Close()) }()
	for _, dir := range repository.Chain(cwd, root) {
		scope := workspaceapp.AgentDocScopeRepositoryRoot
		if dir == cwd {
			scope = workspaceapp.AgentDocScopeCWD
		}
		if err := a.discoverIn(ctx, boundary, dir, dir, scope); err != nil {
			return err
		}
	}
	return nil
}

// A location contributes Flame's own document and the first spelling of the shared agent document.
func (a *agentDocScan) discoverIn(ctx context.Context, root *os.Root, flameParent, sharedDir string, scope workspaceapp.AgentDocScope) error {
	if _, err := a.try(ctx, root, filepath.Join(flameParent, identity.AuthoredDirectory, agentDocName), scope); err != nil {
		return err
	}
	return a.tryFirst(ctx, root, scope,
		filepath.Join(sharedDir, agentDocName),
		filepath.Join(sharedDir, "agents.md"),
	)
}

func openDocumentRoot(path string) (*os.Root, error) {
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(physical)
}

// A duplicate physical source still counts as a match so a first-match group never
// falls through to a lower-precedence alias.
func (a *agentDocScan) try(ctx context.Context, root *os.Root, path string, scope workspaceapp.AgentDocScope) (bool, error) {
	abs, found, err := resolveAgentDocCandidate(ctx, path)
	if err != nil || !found {
		return false, err
	}
	relative, err := filepath.Rel(root.Name(), abs)
	if err != nil || !filepath.IsLocal(relative) {
		return false, fmt.Errorf("%w: repository instruction %q: %w", workspaceapp.ErrInvalidPromptSource, path, workspaceapp.ErrPathOutsideRoot)
	}
	if _, dup := a.seen[abs]; dup {
		return true, nil
	}
	content, size, ok, err := readIfNonEmpty(ctx, root, relative)
	if err != nil {
		return false, fmt.Errorf("promptsource: read agent document: %w", err)
	}
	if !ok {
		return false, nil
	}
	if err := a.admit(abs, content, size, scope); err != nil {
		return false, err
	}
	return true, nil
}

func resolveAgentDocCandidate(ctx context.Context, path string) (string, bool, error) {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return "", false, err
	}
	if path == "" {
		return "", false, nil
	}
	clean := filepath.Clean(path)
	if _, err := os.Lstat(clean); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("promptsource: inspect agent document %q: %w", path, err)
	}
	abs, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", false, fmt.Errorf("promptsource: resolve agent document %q: %w", path, err)
	}
	return abs, true, nil
}

func (a *agentDocScan) admit(path, content string, size int, scope workspaceapp.AgentDocScope) error {
	if len(a.files) >= workspaceapp.MaxAgentDocumentsPerCascade {
		return fmt.Errorf(
			"%w: agent document cascade has more than %d documents",
			workspaceapp.ErrPromptSourceTooLarge,
			workspaceapp.MaxAgentDocumentsPerCascade,
		)
	}
	if size > workspaceapp.MaxAgentDocumentCascadeBytes-a.rawBytes {
		return fmt.Errorf(
			"%w: agent document cascade exceeds %d bytes",
			workspaceapp.ErrPromptSourceTooLarge,
			workspaceapp.MaxAgentDocumentCascadeBytes,
		)
	}
	a.seen[path] = struct{}{}
	a.rawBytes += size
	a.files = append(a.files, workspaceapp.AgentDocFile{Path: path, Content: content, Scope: scope})
	return nil
}

func (a *agentDocScan) tryFirst(ctx context.Context, root *os.Root, scope workspaceapp.AgentDocScope, candidates ...string) error {
	for _, c := range candidates {
		matched, err := a.try(ctx, root, c, scope)
		if err != nil {
			return err
		}
		if matched {
			return nil
		}
	}
	return nil
}

func readIfNonEmpty(ctx context.Context, root *os.Root, path string) (string, int, bool, error) {
	data, err := readAuthoredPromptFile(ctx, root, path)
	if err != nil {
		return "", 0, false, err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", len(data), false, nil
	}
	return content, len(data), true, nil
}
