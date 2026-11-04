package terminal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/kit"
)

type workspaceChoice struct {
	workspace workbench.Workspace
	current   bool
	available bool
	detail    string
}

func (a *app) buildWorkspacePicker(theme kit.Theme, glyphs kit.Glyphs) {
	a.workspaceChoices = newPickerDialog(pickerDialogConfig[workspaceChoice]{
		stack: &a.stack, theme: theme, glyphs: glyphs, title: "Workspaces", search: "search recent workspaces",
		where: layout.Placement{Width: 92, Height: 20},
		label: func(choice workspaceChoice) string { return choice.workspace.Path },
		detail: func(choice workspaceChoice) string {
			if choice.current {
				return "current"
			}
			if choice.detail != "" {
				return choice.detail
			}
			return compactRelativeAge(choice.workspace.LastOpened)
		},
		choose: func(choice workspaceChoice) { a.lifecycle.chooseWorkspaceChoice(choice) },
	})
}

func (l *sessionLifecycle) chooseWorkspaceChoice(choice workspaceChoice) {
	if !choice.available {
		l.root.message("workspace is no longer available · " + choice.workspace.Path)
		return
	}
	if l.workspaces != nil {
		l.resolveAndStartWorkspace(choice.workspace.Path)
		return
	}
	if err := l.createSessionInWorkspace(choice.workspace.Path); err != nil {
		l.root.message(err.Error())
	}
}

func (l *sessionLifecycle) chooseWorkspace(argument string) error {
	if strings.TrimSpace(argument) != "" {
		if l.workspaces != nil {
			l.resolveAndStartWorkspace(argument)
			return nil
		}
		return l.createSessionInWorkspace(argument)
	}
	if l.workspaces != nil {
		l.loadWorkspaceChoices()
		return nil
	}
	return l.showLocalWorkspaceChoices()
}

func (l *sessionLifecycle) showLocalWorkspaceChoices() error {
	workspaces := l.workbench.Workspaces()
	if len(workspaces) == 0 {
		return errors.New("there are no recent workspaces")
	}
	choices := make([]workspaceChoice, 0, len(workspaces))
	for _, workspace := range workspaces {
		choices = append(choices, workspaceChoice{
			workspace: workspace,
			current:   workspace.Path == l.state.current.Workspace.Path,
			available: true,
		})
	}
	l.workspaceChoices.show(choices)
	l.status.note("choose a workspace")
	return nil
}

func (l *sessionLifecycle) loadWorkspaceChoices() {
	l.status.note("loading runtime workspaces")
	currentWorkspace := l.state.current.Workspace.Path
	recentWorkspaces := l.workbench.Workspaces()
	l.tasks.session(workspaceQueryOperation, true,
		func(ctx context.Context) ([]workspaceChoice, error) {
			known, err := l.workspaces.List(ctx)
			if err != nil {
				return nil, err
			}
			return mergeWorkspaceChoices(known, recentWorkspaces, currentWorkspace), nil
		},
		func(choices []workspaceChoice, err error) {
			if err != nil {
				l.root.message("could not load workspaces: " + err.Error())
				return
			}
			if len(choices) == 0 {
				l.root.message("there are no known workspaces")
				return
			}
			l.workspaceChoices.show(choices)
			l.status.note("choose a workspace")
		},
	)
}

func mergeWorkspaceChoices(
	known []workspace.Summary,
	recent []workbench.Workspace,
	currentWorkspace string,
) []workspaceChoice {
	byPath := make(map[string]workspaceChoice, len(known)+len(recent))
	for _, summary := range known {
		lastOpened := time.Time{}
		if summary.LastActive != nil {
			lastOpened = *summary.LastActive
		}
		detail := fmt.Sprintf("%d sessions", summary.Sessions)
		if !summary.Workspace.IsAvailable() {
			detail += " · missing"
		}
		byPath[summary.Workspace.Path] = workspaceChoice{
			workspace: workbench.Workspace{Path: summary.Workspace.Path, LastOpened: lastOpened},
			current:   summary.Workspace.Path == currentWorkspace,
			available: summary.Workspace.IsAvailable(), detail: detail,
		}
	}
	for _, remembered := range recent {
		if _, exists := byPath[remembered.Path]; exists {
			continue
		}
		byPath[remembered.Path] = workspaceChoice{
			workspace: remembered, current: remembered.Path == currentWorkspace, available: true,
		}
	}
	choices := make([]workspaceChoice, 0, len(byPath))
	for _, choice := range byPath {
		choices = append(choices, choice)
	}
	sort.Slice(choices, func(left, right int) bool {
		if choices[left].current != choices[right].current {
			return choices[left].current
		}
		return choices[left].workspace.LastOpened.After(choices[right].workspace.LastOpened)
	})
	return choices
}

func (l *sessionLifecycle) resolveAndStartWorkspace(requested string) {
	path, err := l.resolveWorkspaceInput(requested)
	if err != nil {
		l.root.message(err.Error())
		return
	}
	l.status.note("resolving workspace")
	l.tasks.session(workspaceQueryOperation, true,
		func(ctx context.Context) (workspace.Workspace, error) {
			return l.workspaces.Resolve(ctx, workspace.ResolveRequest{Path: path})
		},
		func(resolved workspace.Workspace, err error) {
			if err != nil {
				l.root.message("resolve workspace: " + err.Error())
				return
			}
			if !resolved.IsAvailable() {
				l.root.message("workspace is unavailable · " + resolved.Path)
				return
			}
			l.startSessionInWorkspace(resolved.Path)
		},
	)
}

func (l *sessionLifecycle) createSessionInWorkspace(requested string) error {
	workspace, err := l.resolveWorkspaceInput(requested)
	if err != nil {
		return err
	}
	l.startSessionInWorkspace(workspace)
	return nil
}

func (l *sessionLifecycle) RelocateSession(requested string) error {
	if err := l.root.requireRuntimeFeature(protocol.FeatureRelocate); err != nil {
		return err
	}
	path, err := l.resolveWorkspaceInput(requested)
	if err != nil {
		return err
	}
	if l.workspaces == nil {
		l.relocateSession(path)
		return nil
	}
	l.status.note("resolving workspace")
	l.tasks.session(workspaceQueryOperation, true,
		func(ctx context.Context) (workspace.Workspace, error) {
			return l.workspaces.Resolve(ctx, workspace.ResolveRequest{Path: path})
		},
		func(resolved workspace.Workspace, err error) {
			if err != nil {
				l.root.message("resolve workspace: " + err.Error())
				return
			}
			if !resolved.IsAvailable() {
				l.root.message("workspace is unavailable · " + resolved.Path)
				return
			}
			l.relocateSession(resolved.Path)
		},
	)
	return nil
}

func (l *sessionLifecycle) relocateSession(path string) {
	if path == l.state.current.Workspace.Path {
		l.root.message("session already uses " + path)
		return
	}
	sessionID := l.state.current.ID
	l.runSessionChange("relocating session",
		func(ctx context.Context) (conversation.SessionSnapshot, error) {
			latest, err := l.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return conversation.SessionSnapshot{}, err
			}
			if _, err := l.runtime.UpdateSession(ctx, conversation.UpdateSession{
				SessionID: sessionID, Workspace: &path, ExpectedRevision: latest.Session.Revision,
			}); err != nil {
				return conversation.SessionSnapshot{}, err
			}
			return l.readSessionAfterMutation(ctx, sessionID)
		},
		func(snapshot conversation.SessionSnapshot) error { return l.installSnapshot(snapshot) },
	)
}

// startSessionInWorkspace accepts an already established workspace identity.
// Callers resolving new user input go through createSessionInWorkspace; /new
// reuses the runtime-authoritative workspace of the current session directly.
func (l *sessionLifecycle) startSessionInWorkspace(workspace string) {
	l.runSessionChange("creating session in "+workspace,
		func(ctx context.Context) (conversation.SessionSnapshot, error) {
			created, err := l.runtime.CreateSession(ctx, conversation.CreateSession{Workspace: workspace})
			return conversation.SessionSnapshot{Session: created}, err
		},
		func(snapshot conversation.SessionSnapshot) error { return l.installSnapshot(snapshot) },
	)
}

// replaceDeletedSessionInWorkspace is the forced counterpart to /new. The
// current draft moves to the replacement because its source identity is gone;
// queued follow-ups remain session-scoped and are retired after installation.
func (l *sessionLifecycle) replaceDeletedSessionInWorkspace(workspace string) {
	retiredSessionID := l.state.current.ID
	l.runSessionChangeWithDraftDisposition("creating replacement session in "+workspace, retireSourceDraft,
		func(ctx context.Context) (conversation.SessionSnapshot, error) {
			created, err := l.runtime.CreateSession(ctx, conversation.CreateSession{Workspace: workspace})
			return conversation.SessionSnapshot{Session: created}, err
		},
		func(snapshot conversation.SessionSnapshot) error {
			if err := l.installSnapshot(snapshot); err != nil {
				return err
			}
			discarded, err := l.retireSessionState(retiredSessionID)
			if err != nil {
				l.root.message("replacement installed; local state cleanup failed: " + err.Error())
				return nil
			}
			if discarded > 0 {
				l.root.message(fmt.Sprintf("discarded %s from the deleted session", countedNoun(discarded, "queued prompt")))
			}
			return nil
		},
	)
}

func resolveWorkspace(current, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", errors.New("workspace path is empty")
	}
	if requested == "~" || strings.HasPrefix(requested, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve workspace home: %w", err)
		}
		if requested == "~" {
			requested = home
		} else {
			requested = home + string(filepath.Separator) + strings.TrimPrefix(requested, "~"+string(filepath.Separator))
		}
	}
	resolved, err := filesystem.ResolvePath(current, requested)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("workspace is not a directory")
	}
	return resolved, nil
}

func (l *sessionLifecycle) resolveWorkspaceInput(requested string) (string, error) {
	if l.localDirectory == "" {
		return resolveWorkspace(l.state.current.Workspace.Path, requested)
	}
	ref := protocol.WorkspaceRef{Path: strings.TrimSpace(requested)}
	if err := protocol.ValidateWireTree(ref); err != nil {
		return "", fmt.Errorf("workspace reference: %w", err)
	}
	return ref.Path, nil
}
