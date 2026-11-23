package sessions

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type Activity string

const (
	ActivityRunning Activity = "running"
	ActivityWaiting Activity = "waiting"
	activityIdle    Activity = "idle"
)

type WorkspaceView struct {
	Path        string
	ProjectRoot string
	Missing     bool
}

type View struct {
	ID              identity.SessionID
	Title           string
	Workspace       WorkspaceView
	Provider        string
	Model           string
	ReasoningEffort string
	Activity        Activity
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Favorite        bool
	Isolated        bool
	Revision        uint64
}

// Reading the durable Run projection rather than the admission gate keeps
// sessions.list linearized with the change notifications.
func (c *Coordinator) Activities(ctx context.Context, sessionIDs []identity.SessionID) (map[identity.SessionID]Activity, error) {
	activities := make(map[identity.SessionID]Activity, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return activities, nil
	}
	requested := make(map[identity.SessionID]struct{}, len(sessionIDs))
	for _, id := range sessionIDs {
		requested[id] = struct{}{}
		activities[id] = activityIdle
	}
	activeRuns, err := c.runs.ListNonTerminalRuns(ctx)
	if err != nil {
		return nil, err
	}
	for _, activeRun := range activeRuns {
		sessionID := activeRun.SessionID()
		if _, ok := requested[sessionID]; !ok {
			continue
		}
		if activeRun.State() == run.Finished {
			return nil, fmt.Errorf(
				"sessions: non-terminal Run read returned finished Run %q",
				activeRun.ID(),
			)
		}
		activities[sessionID] = activities[sessionID].with(activeRun)
	}
	return activities, nil
}

func (a Activity) with(value run.Run) Activity {
	switch value.State() {
	case run.Running:
		return ActivityRunning
	case run.Waiting:
		if a == activityIdle {
			return ActivityWaiting
		}
	case run.Finished:
	}
	return a
}

const viewPageNamespace = "sessions"

const viewPageLimit = 100

const catalogAnchorFields = 3

func catalogAnchorKey(value session.Session) []string {
	key := [catalogAnchorFields]string{strconv.FormatBool(value.Favorite()), strconv.FormatInt(value.UpdatedAt().UnixNano(), 10), value.ID().String()}
	return key[:]
}

func parseCatalogAnchor(key []string) (*session.CatalogAnchor, error) {
	if len(key) == 0 {
		return nil, nil
	}
	if len(key) != catalogAnchorFields {
		return nil, pagination.ErrInvalidCursor
	}
	favorite, err := strconv.ParseBool(key[0])
	if err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	updatedAtNanos, err := strconv.ParseInt(key[1], 10, 64)
	if err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	sessionID, err := identity.ParseSession(key[2])
	if err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	anchor, err := session.NewCatalogAnchor(favorite, time.Unix(0, updatedAtNanos).UTC(), sessionID)
	if err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	return &anchor, nil
}

func (c *Coordinator) ListViewPage(
	ctx context.Context,
	filter session.CatalogFilter,
	cursor string,
	limit pagination.RequestedLimit,
) (pagination.Page[View], error) {
	if err := filter.Validate(); err != nil {
		return pagination.Page[View]{}, err
	}
	filterIdentity := filter.CursorIdentity()
	encodedAnchor, err := pagination.Decode(cursor, viewPageNamespace, filterIdentity)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	after, err := parseCatalogAnchor(encodedAnchor)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	size, err := limit.Resolve(viewPageLimit)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	read, err := session.NewCatalogRead(filter, after, size+1)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	values, err := c.sessions.ListPage(ctx, read)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	bounded, err := pagination.PageOf(values, size, viewPageNamespace, filterIdentity, catalogAnchorKey)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	views, err := c.views(ctx, bounded.Rows)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	return pagination.Page[View]{Rows: views, NextCursor: bounded.NextCursor}, nil
}

func (c *Coordinator) View(ctx context.Context, id identity.SessionID) (View, error) {
	value, err := c.Get(ctx, id)
	if err != nil {
		return View{}, err
	}
	return c.viewForSession(ctx, value)
}

func (c *Coordinator) CreateView(ctx context.Context, title, cwd string) (View, error) {
	creation, err := c.PrepareCreate(ctx, title, cwd)
	if err != nil {
		return View{}, err
	}
	_, err = creation.apply(ctx)
	return creation.View, err
}

func (c *Coordinator) viewForSession(ctx context.Context, value session.Session) (View, error) {
	activities, err := c.Activities(ctx, []identity.SessionID{value.ID()})
	if err != nil {
		return View{}, err
	}
	return c.view(value, activities[value.ID()])
}

func (c *Coordinator) ForkView(ctx context.Context, spec ForkSpec) (View, error) {
	creation, err := c.PrepareFork(ctx, spec)
	if err != nil {
		return View{}, err
	}
	_, err = creation.apply(ctx)
	return creation.View, err
}

func (c *Coordinator) views(ctx context.Context, values []session.Session) ([]View, error) {
	ids := make([]identity.SessionID, len(values))
	for index, value := range values {
		ids[index] = value.ID()
	}
	activities, err := c.Activities(ctx, ids)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(values))
	for _, value := range values {
		view, err := c.view(value, activities[value.ID()])
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (c *Coordinator) view(value session.Session, activity Activity) (View, error) {
	workspace, err := c.InspectWorkspace(value.Workspace().Path())
	if err != nil {
		return View{}, fmt.Errorf("sessions: inspect workspace %q: %w", value.Workspace().Path(), err)
	}
	selection := value.Selection()
	return View{
		ID:    value.ID(),
		Title: value.Title(),
		Workspace: WorkspaceView{
			Path: workspace.Path, ProjectRoot: workspace.ProjectRoot, Missing: workspace.Missing,
		},
		Provider:        selection.Provider(),
		Model:           selection.Model(),
		ReasoningEffort: selection.ReasoningEffort(),
		Activity:        activity,
		CreatedAt:       value.CreatedAt(), UpdatedAt: value.UpdatedAt(),
		Favorite: value.Favorite(), Isolated: value.Isolated(), Revision: value.Revision(),
	}, nil
}
