package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
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

// A Run reads the Project with the workspace whenever a Segment opens, so
// both change only while no Run can observe them.
func (p Patch) changesRunScope() bool {
	return p.WorkspacePath != nil || p.Isolated != nil || p.Project != nil
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

func (c *Coordinator) Create(ctx context.Context, title, cwd string, project identity.ProjectID, complete operation.Reply[View]) error {
	workspace, release, err := c.bindWorkspace(cwd)
	if err != nil {
		return err
	}
	defer release()
	created, err := c.newSession(ctx, session.Draft{ID: c.newID(), Title: title, Workspace: workspace, Project: project})
	if err != nil {
		return err
	}
	view, err := c.view(created, activityIdle)
	if err != nil {
		return err
	}
	if err := complete(ctx, func(tx context.Context) (View, error) {
		return view, c.sessions.Insert(tx, created)
	}); err != nil {
		return err
	}
	c.publishSessionMoved(created.ID())
	return nil
}

func (c *Coordinator) PrepareOpening(ctx context.Context, opening runs.SessionOpening, selection flamemodel.Selection) (session.Session, error) {
	_, err := c.Get(ctx, opening.SessionID)
	if err == nil {
		return session.Session{}, fmt.Errorf("%w: new opening Session already exists", session.ErrInvalid)
	}
	if !errors.Is(err, session.ErrNotFound) {
		return session.Session{}, err
	}
	workspace, err := c.resolveSessionWorkspace(opening.CWD)
	if err != nil {
		return session.Session{}, err
	}
	return c.newSession(ctx, session.Draft{
		ID: opening.SessionID, Title: opening.Title, Workspace: workspace, Project: opening.Project, Selection: selection,
	})
}

func (c *Coordinator) DefaultModelSelection() flamemodel.Selection {
	return c.defaultModelSelection
}

func (c *Coordinator) newSession(ctx context.Context, draft session.Draft) (session.Session, error) {
	if err := c.requireProject(ctx, draft.Project); err != nil {
		return session.Session{}, err
	}
	if !draft.Selection.Configured() {
		draft.Selection = c.defaultModelSelection
	}
	if validateErr := draft.Selection.ValidateExact(); validateErr != nil {
		return session.Session{}, fmt.Errorf("sessions: initial model selection: %w", validateErr)
	}
	if admitErr := c.models.AdmitSelection(ctx, draft.Selection); admitErr != nil {
		return session.Session{}, fmt.Errorf("sessions: initial model selection is not admitted: %w", admitErr)
	}
	draft.CreatedAt = c.now()
	return session.New(draft)
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

func (c *Coordinator) Update(ctx context.Context, id identity.SessionID, patch Patch, complete operation.Reply[View]) error {
	patch = patch.clone()
	var workspace *session.Workspace
	if patch.WorkspacePath != nil {
		resolved, release, err := c.bindWorkspace(*patch.WorkspacePath)
		if err != nil {
			return err
		}
		defer release()
		workspace = &resolved
	}
	if patch.Project != nil {
		if err := c.requireProject(ctx, *patch.Project); err != nil {
			return err
		}
	}
	if patch.changesRunScope() {
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
	if !current.SharesExecutionPolicy(updated) {
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

// A committed execution-policy change must not inherit processes, context evidence
// or an isolated copy from the preceding policy, so those are retired before
// persistence exposes the replacement.
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

func (c *Coordinator) bindWorkspace(path string) (session.Workspace, func(), error) {
	release, acquired, err := c.admissions.AcquireWorkspaceBinding(path)
	if err != nil {
		return session.Workspace{}, nil, errors.Join(workspaceapp.ErrCWDUnavailable, err)
	}
	if !acquired {
		return session.Workspace{}, nil, fmt.Errorf("%w: %q is being released", workspaceapp.ErrCWDUnavailable, path)
	}
	// Keep the checkout present from directory validation through the Session commit.
	workspace, err := c.resolveSessionWorkspace(path)
	if err != nil {
		release()
		return session.Workspace{}, nil, err
	}
	return workspace, release, nil
}
