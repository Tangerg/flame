package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// workspaceResolver is the filesystem boundary needed when a Session is
// created or relocated. Session.Workspace is exact after admission, so
// downstream use cases treat it as an invariant instead of repeatedly touching
// the filesystem.
type workspaceResolver interface {
	ResolveExistingDir(path string) (string, error)
	Inspect(path string) (workspaceapp.Resolved, error)
}

// Patch is the Application command for editing a Session. WorkspacePath is an
// untrusted external spelling until Update admits it through WorkspaceResolver;
// the Domain patch receives only the resulting exact Workspace value.
type Patch struct {
	Title            *string
	ModelSelection   modelchoice.Patch
	WorkspacePath    *string
	Favorite         *bool
	Isolated         *bool
	ExpectedRevision uint64
}

func (p Patch) clone() Patch {
	if p.Title != nil {
		p.Title = new(*p.Title)
	}
	if p.ModelSelection.Provider != nil {
		p.ModelSelection.Provider = new(*p.ModelSelection.Provider)
	}
	if p.ModelSelection.Model != nil {
		p.ModelSelection.Model = new(*p.ModelSelection.Model)
	}
	if p.ModelSelection.ReasoningEffort != nil {
		p.ModelSelection.ReasoningEffort = new(*p.ModelSelection.ReasoningEffort)
	}
	if p.WorkspacePath != nil {
		p.WorkspacePath = new(*p.WorkspacePath)
	}
	if p.Favorite != nil {
		p.Favorite = new(*p.Favorite)
	}
	if p.Isolated != nil {
		p.Isolated = new(*p.Isolated)
	}
	return p
}

// List returns every user-facing Session, newest-updated first.
func (c *Coordinator) List(ctx context.Context) ([]session.Session, error) {
	return c.sessions.List(ctx)
}

// Get returns the saved Session with id.
func (c *Coordinator) Get(ctx context.Context, id identity.SessionID) (session.Session, error) {
	return c.sessions.Get(ctx, id)
}

// InspectWorkspace resolves the live filesystem projection of an admitted cwd.
// Missing directories are represented in the returned value; unexpected
// filesystem failures remain errors so callers never receive a fabricated
// workspace identity.
func (c *Coordinator) InspectWorkspace(cwd string) (workspaceapp.Resolved, error) {
	return c.paths.Inspect(cwd)
}

// Create starts and persists a fresh root Session in an admitted workspace.
func (c *Coordinator) Create(ctx context.Context, title, cwd string) (session.Session, error) {
	creation, err := c.PrepareCreate(ctx, title, cwd)
	if err != nil {
		return session.Session{}, err
	}
	return creation.apply(ctx)
}

// Creation separates provider/workspace admission from the short database
// write. A keyed command commits its original view alongside this write;
// invalidations become visible only after that transaction succeeds.
type Creation struct {
	value   session.Session
	View    View
	write   func(context.Context) error
	publish func()
}

func (c Creation) Commit(ctx context.Context) error { return c.write(ctx) }
func (c Creation) Publish()                         { c.publish() }
func (c Creation) apply(ctx context.Context) (session.Session, error) {
	if err := c.Commit(ctx); err != nil {
		return session.Session{}, err
	}
	c.Publish()
	return c.value, nil
}

func (c *Coordinator) PrepareCreate(ctx context.Context, title, cwd string) (Creation, error) {
	created, _, err := c.prepareInitial(ctx, c.newID(), title, cwd, modelchoice.Selection{})
	if err != nil {
		return Creation{}, err
	}
	view, err := c.view(created, activityIdle)
	if err != nil {
		return Creation{}, err
	}
	return Creation{value: created, View: view, write: func(tx context.Context) error { return c.sessions.Insert(tx, created) }, publish: func() { c.publishSessionMoved(created.ID()) }}, nil
}

// PrepareOpening creates a captured Session without writing it. An existing
// identity is refused: only the original Run acceptance may authorize replay.
func (c *Coordinator) PrepareOpening(
	ctx context.Context,
	id identity.SessionID,
	title, cwd string,
	selection modelchoice.Selection,
) (current session.Session, initial *session.Session, err error) {
	_, err = c.Get(ctx, id)
	if err == nil {
		return session.Session{}, nil, fmt.Errorf("%w: new opening Session already exists", session.ErrInvalid)
	}
	if !errors.Is(err, session.ErrNotFound) {
		return session.Session{}, nil, err
	}
	return c.prepareInitial(ctx, id, title, cwd, selection)
}

// DefaultModelSelection is the selection a Session created without one runs
// on. Clients read it instead of choosing a stand-in model of their own.
func (c *Coordinator) DefaultModelSelection() modelchoice.Selection {
	return c.defaultModelSelection
}

func (c *Coordinator) prepareInitial(
	ctx context.Context,
	id identity.SessionID,
	title string,
	cwd string,
	selection modelchoice.Selection,
) (session.Session, *session.Session, error) {
	workspace, err := c.resolveSessionWorkspace(cwd)
	if err != nil {
		return session.Session{}, nil, err
	}
	if !selection.Configured() {
		selection = c.defaultModelSelection
	}
	if validateErr := selection.ValidateExact(); validateErr != nil {
		return session.Session{}, nil, fmt.Errorf("sessions: initial model selection: %w", validateErr)
	}
	if admitErr := c.models.AdmitSelection(ctx, selection); admitErr != nil {
		return session.Session{}, nil, fmt.Errorf("sessions: initial model selection is not admitted: %w", admitErr)
	}
	created, err := session.New(session.Draft{
		ID: id, Title: title, Workspace: workspace, Selection: selection, CreatedAt: c.now(),
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
func (c *Coordinator) patchedWorkspace(path *string) (*session.Workspace, error) {
	if path == nil {
		return nil, nil
	}
	resolved, err := c.resolveSessionWorkspace(*path)
	if err != nil {
		return nil, err
	}
	return &resolved, nil
}

func (c *Coordinator) Update(ctx context.Context, id identity.SessionID, patch Patch, complete operation.Reply[View]) error {
	patch = patch.clone()
	workspace, err := c.patchedWorkspace(patch.WorkspacePath)
	if err != nil {
		return err
	}
	if workspace != nil || patch.Isolated != nil {
		admission, err := c.ClaimIdleSession(ctx, id)
		if err != nil {
			return err
		}
		defer admission.Release()
	}
	current, err := c.Get(ctx, id)
	if err != nil {
		return err
	}
	selection, err := c.admittedSelection(ctx, patch.ModelSelection, current.Selection())
	if err != nil {
		return err
	}
	updated, changed, err := current.Edit(patch.ExpectedRevision, session.Patch{
		Title: patch.Title, Selection: selection, Workspace: workspace,
		Favorite: patch.Favorite, Isolated: patch.Isolated,
	}, c.now())
	if err != nil {
		return err
	}
	if !changed {
		return complete(ctx, func(tx context.Context) (View, error) {
			return c.viewForSession(tx, current)
		})
	}
	replacement, err := session.NextReplacement(current, updated)
	if err != nil {
		return fmt.Errorf("sessions: prepare Session replacement: %w", err)
	}
	if current.Workspace() != updated.Workspace() || current.Isolated() != updated.Isolated() {
		if err := c.retireExecutionPolicy(id); err != nil {
			return err
		}
	}
	if err := complete(ctx, func(tx context.Context) (View, error) {
		view, err := c.viewForSession(tx, updated)
		if err != nil {
			return View{}, err
		}
		return view, c.sessions.Save(tx, replacement)
	}); err != nil {
		return err
	}
	c.publishSessionMoved(id)
	return nil
}

// admittedSelection resolves a selection patch against the Session's current
// choice; an empty patch keeps it.
func (c *Coordinator) admittedSelection(ctx context.Context, patch modelchoice.Patch, current modelchoice.Selection) (*modelchoice.Selection, error) {
	if patch.Empty() {
		return nil, nil
	}
	resolved, err := patch.Apply(current)
	if err != nil {
		return nil, err
	}
	if err := c.models.AdmitSelection(ctx, resolved); err != nil {
		return nil, fmt.Errorf("sessions: model selection is not admitted: %w", err)
	}
	return &resolved, nil
}

// retireExecutionPolicy drops every process-local fact bound to the old
// workspace or isolation before the Session moves.
func (c *Coordinator) retireExecutionPolicy(id identity.SessionID) error {
	if err := c.transientState.QuiesceSession(id); err != nil {
		return fmt.Errorf("sessions: quiesce process-local Session state before execution policy change: %w", err)
	}
	if err := c.sandbox.Discard(id); err != nil {
		return fmt.Errorf("sessions: discard sandbox copy before execution policy change: %w", err)
	}
	// This cache is optional, derived state. Retiring it before the CAS is
	// conservative: a conflicting durable edit can rebuild the old policy's
	// context, while carrying evidence across a successful relocation is unsafe.
	c.transientState.ForgetSessionContext(id)
	return nil
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
