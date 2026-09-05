package sessions

import (
	"context"
	"errors"
	"fmt"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// WorkspaceResolver is the filesystem boundary needed when a Session is
// created or relocated. Session.Workspace is exact after admission, so
// downstream use cases treat it as an invariant instead of repeatedly touching
// the filesystem.
type WorkspaceResolver interface {
	ResolveExistingDir(path string) (string, error)
	Inspect(path string) (workspaceapp.Resolved, error)
}

// Patch is the Application command for editing a Session. WorkspacePath is an
// untrusted external spelling until Update admits it through WorkspaceResolver;
// the Domain patch receives only the resulting exact Workspace value.
type Patch struct {
	Title            *string
	ModelSelection   modelref.Patch
	WorkspacePath    *string
	Favorite         *bool
	Isolated         *bool
	ExpectedRevision uint64
}

func (p Patch) clone() Patch {
	p.Title = clonePatchValue(p.Title)
	p.ModelSelection.Provider = clonePatchValue(p.ModelSelection.Provider)
	p.ModelSelection.Model = clonePatchValue(p.ModelSelection.Model)
	p.ModelSelection.ReasoningEffort = clonePatchValue(p.ModelSelection.ReasoningEffort)
	p.WorkspacePath = clonePatchValue(p.WorkspacePath)
	p.Favorite = clonePatchValue(p.Favorite)
	p.Isolated = clonePatchValue(p.Isolated)
	return p
}

func clonePatchValue[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// List returns every user-facing Session, newest-updated first.
func (c *Coordinator) List(ctx context.Context) ([]session.Session, error) {
	values, err := c.sessions.List(ctx)
	if err != nil {
		return nil, err
	}
	if err := session.ValidateCatalog(values); err != nil {
		return nil, err
	}
	return values, nil
}

// Get returns one valid saved Session whose identity exactly matches id.
func (c *Coordinator) Get(ctx context.Context, id string) (session.Session, error) {
	value, err := c.sessions.Get(ctx, id)
	if err != nil {
		return session.Session{}, err
	}
	if err := value.ValidateFor(id); err != nil {
		return session.Session{}, fmt.Errorf("sessions: read session %q: %w", id, err)
	}
	return value, nil
}

// InspectWorkspace resolves the live filesystem projection of an admitted cwd.
// Missing directories are represented in the returned value; unexpected
// filesystem failures remain errors so callers never receive a fabricated
// workspace identity.
func (c *Coordinator) InspectWorkspace(cwd string) (workspaceapp.Resolved, error) {
	resolved, err := c.paths.Inspect(cwd)
	if err != nil {
		return workspaceapp.Resolved{}, err
	}
	if err := resolved.Validate(); err != nil {
		return workspaceapp.Resolved{}, err
	}
	return resolved, nil
}

// Create starts and persists a fresh root Session in an admitted workspace.
func (c *Coordinator) Create(ctx context.Context, title, cwd string) (session.Session, error) {
	workspace, err := c.resolveSessionWorkspace(cwd)
	if err != nil {
		return session.Session{}, err
	}
	created, err := session.New(session.Draft{
		ID: c.newID(), Title: title, Workspace: workspace,
		Selection: c.defaultModelSelection, StartedAt: c.now(),
	})
	if err != nil {
		return session.Session{}, err
	}
	if err := c.sessions.Insert(ctx, created); err != nil {
		return session.Session{}, err
	}
	c.publishSessionMoved(created.ID())
	return created, nil
}

// PrepareScheduled resolves a schedule-owned Session without writing it. When
// the identity already exists, it returns that durable aggregate and no initial
// value. Otherwise it returns one initial aggregate for the Run opening write-set.
// This makes a retried occurrence reuse its Session without asking persistence
// to derive or normalize product state.
func (c *Coordinator) PrepareScheduled(
	ctx context.Context,
	id, title, cwd string,
	selection modelref.Selection,
) (current session.Session, initial *session.Session, err error) {
	existing, err := c.Get(ctx, id)
	if err == nil {
		return existing, nil, nil
	}
	if !errors.Is(err, session.ErrNotFound) {
		return session.Session{}, nil, err
	}
	workspace, err := c.resolveSessionWorkspace(cwd)
	if err != nil {
		return session.Session{}, nil, err
	}
	if !selection.Configured() {
		selection = c.defaultModelSelection
	}
	if validateErr := selection.ValidateExact(); validateErr != nil {
		return session.Session{}, nil, fmt.Errorf("sessions: scheduled model selection: %w", validateErr)
	}
	if admitErr := c.models.AdmitSelection(selection); admitErr != nil {
		return session.Session{}, nil, fmt.Errorf("sessions: scheduled model selection is not admitted: %w", admitErr)
	}
	created, err := session.New(session.Draft{
		ID: id, Title: title, Workspace: workspace, Selection: selection, StartedAt: c.now(),
	})
	if err != nil {
		return session.Session{}, nil, err
	}
	return created, &created, nil
}

// Update applies one complete Session edit. External workspace admission and
// process-local mutation claims occur before Domain behavior. A committed
// execution-policy change must not inherit processes, context evidence, or an
// isolated copy from the preceding policy, so those resources are retired
// before persistence exposes the replacement.
func (c *Coordinator) Update(ctx context.Context, id string, patch Patch) (session.Session, error) {
	patch = patch.clone()
	var workspace *session.Workspace
	if patch.WorkspacePath != nil {
		resolved, err := c.resolveSessionWorkspace(*patch.WorkspacePath)
		if err != nil {
			return session.Session{}, err
		}
		workspace = &resolved
	}
	if workspace != nil || patch.Isolated != nil {
		admission, err := c.ClaimIdleSession(ctx, id)
		if err != nil {
			return session.Session{}, err
		}
		defer admission.Release()
	}
	current, err := c.Get(ctx, id)
	if err != nil {
		return session.Session{}, err
	}
	var selection *modelref.Selection
	if !patch.ModelSelection.Empty() {
		resolved, selectionErr := patch.ModelSelection.Apply(current.Selection())
		if selectionErr != nil {
			return session.Session{}, selectionErr
		}
		if selectionErr = c.models.AdmitSelection(resolved); selectionErr != nil {
			return session.Session{}, fmt.Errorf("sessions: model selection is not admitted: %w", selectionErr)
		}
		selection = &resolved
	}
	updated, changed, err := current.Apply(session.Patch{
		Title: patch.Title, Selection: selection, Workspace: workspace,
		Favorite: patch.Favorite, Isolated: patch.Isolated,
		ExpectedRevision: patch.ExpectedRevision,
	}, c.now())
	if err != nil {
		return session.Session{}, err
	}
	if !changed {
		return current, nil
	}
	replacement, err := session.NextReplacement(current, updated)
	if err != nil {
		return session.Session{}, fmt.Errorf("sessions: prepare Session replacement: %w", err)
	}
	executionPolicyChanged := current.Workspace() != updated.Workspace() || current.Isolated() != updated.Isolated()
	if executionPolicyChanged {
		if err := c.transientState.QuiesceSession(id); err != nil {
			return session.Session{}, fmt.Errorf("sessions: quiesce process-local Session state before execution policy change: %w", err)
		}
		if c.sandbox != nil {
			if err := c.sandbox.Discard(id); err != nil {
				return session.Session{}, fmt.Errorf("sessions: discard sandbox copy before execution policy change: %w", err)
			}
		}
		// This cache is optional, derived state. Retiring it before the CAS is
		// conservative: a conflicting durable edit can rebuild the old policy's
		// context, while carrying evidence across a successful relocation is unsafe.
		c.transientState.ForgetSessionContext(id)
	}
	if err := c.sessions.Save(ctx, replacement); err != nil {
		return session.Session{}, err
	}
	c.publishSessionMoved(id)
	return updated, nil
}

// NeedsGeneratedTitle reports whether title generation can still contribute a
// value. The later write rechecks this condition, so a user rename between the
// query and generated result always wins.
func (c *Coordinator) NeedsGeneratedTitle(ctx context.Context, id string) (bool, error) {
	value, err := c.Get(ctx, id)
	if err != nil {
		return false, err
	}
	return value.Title() == "", nil
}

// ApplyGeneratedTitle installs a generated title using Domain first-writer
// semantics. Concurrent Session edits are retried from their committed value;
// once a user title exists, the generated title becomes a successful no-op.
func (c *Coordinator) ApplyGeneratedTitle(ctx context.Context, id, title string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := c.Get(ctx, id)
		if err != nil {
			return err
		}
		updated, changed, err := current.NameIfUntitled(title, c.now())
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		replacement, err := session.NextReplacement(current, updated)
		if err != nil {
			return fmt.Errorf("sessions: prepare generated title replacement: %w", err)
		}
		if err := c.sessions.Save(ctx, replacement); err == nil {
			c.publishSessionMoved(id)
			return nil
		} else if !errors.Is(err, session.ErrRevisionConflict) {
			return fmt.Errorf("sessions: save generated title: %w", err)
		}
	}
}

// resolveSessionWorkspace canonicalizes path and requires it to be an existing
// directory, returning the shared workspace-admission sentinel on failure.
func (c *Coordinator) resolveSessionWorkspace(path string) (session.Workspace, error) {
	resolved, err := c.paths.ResolveExistingDir(path)
	if err != nil {
		return session.Workspace{}, errors.Join(workspaceapp.ErrCWDUnavailable, err)
	}
	workspace, err := session.NewWorkspace(resolved)
	if err != nil {
		return session.Workspace{}, fmt.Errorf("sessions: resolved workspace: %w", err)
	}
	return workspace, nil
}
