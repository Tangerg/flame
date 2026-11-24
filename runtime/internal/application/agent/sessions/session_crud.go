package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type workspaceResolver interface {
	ResolveExistingDir(path string) (string, error)
	Inspect(path string) (workspaceapp.Resolved, error)
}

type Patch struct {
	Title            *string
	ModelSelection   flamemodel.Patch
	WorkspacePath    *string
	Project          *identity.ProjectID
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
	if p.Project != nil {
		p.Project = new(*p.Project)
	}
	if p.Favorite != nil {
		p.Favorite = new(*p.Favorite)
	}
	if p.Isolated != nil {
		p.Isolated = new(*p.Isolated)
	}
	return p
}

func (c *Coordinator) List(ctx context.Context) ([]session.Session, error) {
	return c.sessions.List(ctx)
}

func (c *Coordinator) Get(ctx context.Context, id identity.SessionID) (session.Session, error) {
	return c.sessions.Get(ctx, id)
}

func (c *Coordinator) InspectWorkspace(cwd string) (workspaceapp.Resolved, error) {
	return c.paths.Inspect(cwd)
}

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

func (c *Coordinator) PrepareCreate(ctx context.Context, title, cwd string, project identity.ProjectID) (Creation, error) {
	created, _, err := c.prepareInitial(ctx, c.newID(), title, cwd, project, flamemodel.Selection{})
	if err != nil {
		return Creation{}, err
	}
	view, err := c.view(created, activityIdle)
	if err != nil {
		return Creation{}, err
	}
	return Creation{value: created, View: view, write: func(tx context.Context) error { return c.sessions.Insert(tx, created) }, publish: func() { c.publishSessionMoved(created.ID()) }}, nil
}

func (c *Coordinator) PrepareOpening(
	ctx context.Context,
	id identity.SessionID,
	title, cwd string,
	project identity.ProjectID,
	selection flamemodel.Selection,
) (current session.Session, initial *session.Session, err error) {
	_, err = c.Get(ctx, id)
	if err == nil {
		return session.Session{}, nil, fmt.Errorf("%w: new opening Session already exists", session.ErrInvalid)
	}
	if !errors.Is(err, session.ErrNotFound) {
		return session.Session{}, nil, err
	}
	return c.prepareInitial(ctx, id, title, cwd, project, selection)
}

func (c *Coordinator) DefaultModelSelection() flamemodel.Selection {
	return c.defaultModelSelection
}

func (c *Coordinator) prepareInitial(
	ctx context.Context,
	id identity.SessionID,
	title string,
	cwd string,
	project identity.ProjectID,
	selection flamemodel.Selection,
) (session.Session, *session.Session, error) {
	workspace, err := c.resolveSessionWorkspace(cwd)
	if err != nil {
		return session.Session{}, nil, err
	}
	if err := c.requireProject(ctx, project); err != nil {
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
		ID: id, Title: title, Workspace: workspace, Project: project, Selection: selection, CreatedAt: c.now(),
	})
	if err != nil {
		return session.Session{}, nil, err
	}
	return created, &created, nil
}

// Projects are never deleted, so an existence check before the write cannot be
// invalidated by the time the membership commits.
func (c *Coordinator) requireProject(ctx context.Context, id identity.ProjectID) error {
	if id.IsZero() {
		return nil
	}
	_, err := c.projects.Get(ctx, id)
	return err
}

// A committed execution-policy change must not inherit processes, context evidence
// or an isolated copy from the preceding policy, so those are retired before
// persistence exposes the replacement.
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
	if patch.Project != nil {
		if err := c.requireProject(ctx, *patch.Project); err != nil {
			return err
		}
	}
	// A Run reads the Project with the workspace whenever a Segment opens, so
	// both change only while no Run can observe them.
	if workspace != nil || patch.Isolated != nil || patch.Project != nil {
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
		Project: patch.Project, Favorite: patch.Favorite, Isolated: patch.Isolated,
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

func (c *Coordinator) admittedSelection(ctx context.Context, patch flamemodel.Patch, current flamemodel.Selection) (*flamemodel.Selection, error) {
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

func (c *Coordinator) retireExecutionPolicy(id identity.SessionID) error {
	if err := c.transientState.QuiesceSession(id); err != nil {
		return fmt.Errorf("sessions: quiesce process-local Session state before execution policy change: %w", err)
	}
	if err := c.sandbox.Discard(id); err != nil {
		return fmt.Errorf("sessions: discard sandbox copy before execution policy change: %w", err)
	}
	c.transientState.ForgetSessionContext(id)
	return nil
}

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
