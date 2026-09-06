// Package promptsource is the filesystem adapter for the prompt-source domains:
// it discovers the AGENTS.md, skill, and recipe files a session exposes, walking
// the project tree and the well-known user-level directories. The precedence,
// render, and parse RULES are the domains' (agentdoc / skills / recipes); the
// file discovery and reads are here.
package promptsource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
)

// DiscoverAgentDocs walks the project tree + user-level locations and returns
// the AGENTS.md files in render order:
//
//  1. ~/.flame/AGENTS.md           (Flame-specific user scope)
//  2. ~/.agents/AGENTS.md         (cross-tool generic; first match
//     of AGENTS.md / agents.md)
//  3. for each dir from project-root → cwd inclusive:
//     - {dir}/.flame/AGENTS.md     (Flame subdir convention)
//     - {dir}/AGENTS.md           (first match of AGENTS.md / agents.md)
//
// Project root = the nearest ancestor containing a `.git` entry; if none is
// found, root = cwd (single-level scan). Symlinked duplicate paths are deduped
// by resolved absolute path.
//
// ctx cancels long walks; cwd / home are absolute composition inputs. Missing
// and empty files contribute nothing. An existing unreadable, invalid, or
// oversized document rejects the complete cascade so management and execution
// cannot observe different instruction sets. The agent-execution adapter
// renders the resulting values.
func DiscoverAgentDocs(ctx context.Context, cwd, home string) ([]workspaceapp.AgentDocFile, error) {
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
	if err := d.discoverProjectTree(ctx, cwd); err != nil {
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

// AgentDocs adapts prompt-source discovery to the workspace application port.
type AgentDocs struct{}

func (AgentDocs) Find(ctx context.Context, cwd, home string) ([]workspaceapp.AgentDocFile, error) {
	return DiscoverAgentDocs(ctx, cwd, home)
}

// agentDocScan carries de-duplication and complete-cascade admission across the
// walk. Missing/empty candidates remain absent; an existing invalid candidate
// returns its error instead of silently changing the instruction set.
type agentDocScan struct {
	seen     map[string]struct{}
	files    []workspaceapp.AgentDocFile
	rawBytes int
}

// discoverHome applies the user-level order: Flame-specific first, then the
// first non-empty generic candidate.
func (a *agentDocScan) discoverHome(ctx context.Context, home string) error {
	if home == "" {
		return nil
	}
	if _, err := a.try(ctx, filepath.Join(home, ".flame", "AGENTS.md"), workspaceapp.AgentDocScopeHome); err != nil {
		return err
	}
	return a.tryFirst(ctx, workspaceapp.AgentDocScopeHome,
		filepath.Join(home, ".agents", "AGENTS.md"),
		filepath.Join(home, ".agents", "agents.md"),
	)
}

// discoverProjectTree walks root to leaf so the most specific files remain at
// the end of the cascade consumed by prompt assembly.
func (a *agentDocScan) discoverProjectTree(ctx context.Context, cwd string) error {
	root := findProjectRoot(cwd)
	for _, dir := range dirsRootToLeaf(cwd, root) {
		scope := workspaceapp.AgentDocScopeProjectRoot
		if dir == cwd {
			scope = workspaceapp.AgentDocScopeCWD
		}
		if _, err := a.try(ctx, filepath.Join(dir, ".flame", "AGENTS.md"), scope); err != nil {
			return err
		}
		if err := a.tryFirst(ctx, scope,
			filepath.Join(dir, "AGENTS.md"),
			filepath.Join(dir, "agents.md"),
		); err != nil {
			return err
		}
	}
	return nil
}

// try reports whether a non-empty candidate matched. A duplicate physical
// source still counts as a match so a first-match group never falls through to
// a lower-precedence alias merely because the source was already admitted.
func (a *agentDocScan) try(ctx context.Context, path string, scope workspaceapp.AgentDocScope) (bool, error) {
	abs, found, err := resolveAgentDocCandidate(ctx, path)
	if err != nil || !found {
		return false, err
	}
	if _, dup := a.seen[abs]; dup {
		return true, nil
	}
	content, size, ok, err := readIfNonEmpty(ctx, abs)
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
	if err := ctx.Err(); err != nil {
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

func (a *agentDocScan) tryFirst(ctx context.Context, scope workspaceapp.AgentDocScope, candidates ...string) error {
	for _, c := range candidates {
		matched, err := a.try(ctx, c, scope)
		if err != nil {
			return err
		}
		if matched {
			return nil
		}
	}
	return nil
}

// readIfNonEmpty returns trimmed, admitted content and its complete encoded
// size. Empty files remain absent; an existing invalid file remains observable.
func readIfNonEmpty(ctx context.Context, path string) (string, int, bool, error) {
	data, err := readAuthoredPromptFile(ctx, path)
	if err != nil {
		return "", 0, false, err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", len(data), false, nil
	}
	return content, len(data), true, nil
}

// findProjectRoot walks up from cwd looking for a `.git` entry (dir OR file —
// submodules use `.git` files pointing to the real gitdir). Returns cwd unchanged
// if no .git is found anywhere on the way up (single-dir scan).
func findProjectRoot(cwd string) string {
	current := cwd
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return cwd
		}
		current = parent
	}
}

// dirsRootToLeaf returns the chain [root, ..., cwd] (inclusive at both ends).
// When root == cwd the slice has one element.
func dirsRootToLeaf(cwd, root string) []string {
	if cwd == root {
		return []string{cwd}
	}
	var chain []string
	current := cwd
	for current != root {
		chain = append(chain, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	chain = append(chain, root)
	slices.Reverse(chain)
	return chain
}
