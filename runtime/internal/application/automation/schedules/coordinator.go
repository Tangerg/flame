// Package schedules owns cron-triggered headless-run management and firing.
// Management is independent from execution; firing is built after Runs without
// mutable post-construction wiring.
package schedules

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

// ManagementStore is the editable-schedule persistence slice owned by this
// use case. Firing and worker cursor updates intentionally remain separate.
type ManagementStore interface {
	ListPage(ctx context.Context, afterCreatedAt time.Time, afterID string, limit int) ([]schedule.Schedule, error)
	Get(ctx context.Context, id string) (schedule.Schedule, error)
	Insert(ctx context.Context, scheduled schedule.Schedule) error
	Update(ctx context.Context, scheduled schedule.Schedule, expectedRevision uint64) error
	Delete(ctx context.Context, id string) (bool, error)
}

type scheduleReader interface {
	Get(ctx context.Context, id string) (schedule.Schedule, error)
}

// Coordinator owns editable scheduled-run management over its narrow store.
// It is stateless beyond its dependencies and safe to share.
type Coordinator struct {
	store         ManagementStore
	paths         CWDResolver
	models        ModelAdmitter
	newScheduleID func() string
	now           func() time.Time
	invalidations invalidation.Publish
}

// CWDResolver is the filesystem boundary used to admit a schedule's working
// directory. Persisted schedules always hold either an empty cwd (the runtime
// default) or a canonical existing directory.
type CWDResolver interface {
	ResolveExistingDir(path string) (string, error)
}

// ModelAdmitter validates an exact schedule-owned model choice before it is
// persisted or captured by a firing.
type ModelAdmitter interface {
	AdmitSelection(selection modelref.Selection) error
}

// ErrUnavailable reports that scheduled-run management was not assembled in
// this Runtime.
var ErrUnavailable = errors.New("schedules: unavailable")

// Dependencies is the collaborator set [New] wires into a Coordinator.
type Dependencies struct {
	Store         ManagementStore
	Paths         CWDResolver
	Models        ModelAdmitter
	NewScheduleID func() string
	Invalidations invalidation.Publish
}

// CreateCommand is the complete editable state of a new schedule.
type CreateCommand struct {
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelref.Selection
	Cron           string
	Enabled        bool
}

// UpdateCommand applies a partial edit to one stored schedule.
type UpdateCommand struct {
	ID               string
	Patch            schedule.Patch
	ModelSelection   modelref.Patch
	ExpectedRevision uint64
}

// Disabled returns an explicitly unavailable schedule-management capability.
func Disabled() *Coordinator { return &Coordinator{} }

// New returns a fully wired Coordinator or rejects partial construction.
func New(deps Dependencies) (*Coordinator, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{name: "store", value: deps.Store},
		{name: "cwd resolver", value: deps.Paths},
		{name: "model admitter", value: deps.Models},
		{name: "schedule identity factory", value: deps.NewScheduleID},
	} {
		if dependencyMissing(required.value) {
			return nil, fmt.Errorf("schedules: %s is required", required.name)
		}
	}
	return &Coordinator{
		store:         deps.Store,
		paths:         deps.Paths,
		models:        deps.Models,
		newScheduleID: deps.NewScheduleID,
		now:           time.Now,
		invalidations: deps.Invalidations,
	}, nil
}

// Available reports whether schedule-management use cases are wired.
func (c *Coordinator) Available() bool { return c != nil && c.store != nil }

// listPageNamespace binds cursors to this schedule read independently of other
// paged reads.
const listPageNamespace = "schedules"

// listPageLimit is the widest schedule page this read will serve.
const listPageLimit = 100

// ListPage returns one page of schedules, newest-created first, continuing after
// cursor.
func (c *Coordinator) ListPage(ctx context.Context, cursor string, limit pagination.RequestedLimit) (pagination.Page[schedule.Schedule], error) {
	anchor, err := pagination.Decode(cursor, listPageNamespace, nil)
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	var afterCreatedAt time.Time
	var afterID string
	if len(anchor) > 0 {
		if len(anchor) != 2 {
			return pagination.Page[schedule.Schedule]{}, pagination.ErrInvalidCursor
		}
		afterCreatedAtNanos, parseErr := strconv.ParseInt(anchor[0], 10, 64)
		if parseErr != nil {
			return pagination.Page[schedule.Schedule]{}, pagination.ErrInvalidCursor
		}
		afterCreatedAt = time.Unix(0, afterCreatedAtNanos).UTC()
		afterID = anchor[1]
		if err := schedule.ValidateID(afterID); err != nil {
			return pagination.Page[schedule.Schedule]{}, pagination.ErrInvalidCursor
		}
	}
	size, err := limit.Resolve(listPageLimit)
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	if !c.Available() {
		return pagination.Page[schedule.Schedule]{}, ErrUnavailable
	}
	rows, err := c.store.ListPage(ctx, afterCreatedAt, afterID, size+1)
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	if err := validateManagementPage(rows, afterCreatedAt, afterID, size+1); err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	rows = slices.Clone(rows)
	return pagination.PageOf(rows, size, listPageNamespace, nil, func(scheduled schedule.Schedule) []string {
		return []string{strconv.FormatInt(scheduled.CreatedAt().UnixNano(), 10), scheduled.ID()}
	})
}

func validateManagementPage(rows []schedule.Schedule, afterCreatedAt time.Time, afterID string, maximum int) error {
	if len(rows) > maximum {
		return fmt.Errorf("schedules: store returned %d rows, maximum %d", len(rows), maximum)
	}
	seen := make(map[string]struct{}, len(rows))
	for index, scheduled := range rows {
		if err := scheduled.ValidateStored(); err != nil {
			return fmt.Errorf("schedules: store row %d is invalid: %w", index+1, err)
		}
		if _, duplicate := seen[scheduled.ID()]; duplicate {
			return fmt.Errorf("schedules: store page repeats schedule %q", scheduled.ID())
		}
		seen[scheduled.ID()] = struct{}{}
		if (!afterCreatedAt.IsZero() || afterID != "") &&
			(scheduled.CreatedAt().After(afterCreatedAt) ||
				scheduled.CreatedAt().Equal(afterCreatedAt) && scheduled.ID() >= afterID) {
			return fmt.Errorf("schedules: store row %q does not follow the page cursor", scheduled.ID())
		}
		if index == 0 {
			continue
		}
		previous := rows[index-1]
		if scheduled.CreatedAt().After(previous.CreatedAt()) ||
			scheduled.CreatedAt().Equal(previous.CreatedAt()) && scheduled.ID() >= previous.ID() {
			return fmt.Errorf("schedules: store row %q is out of order after %q", scheduled.ID(), previous.ID())
		}
	}
	return nil
}

// Create validates, normalizes, schedules, and persists a new schedule.
func (c *Coordinator) Create(ctx context.Context, cmd CreateCommand) (schedule.Schedule, error) {
	if !c.Available() {
		return schedule.Schedule{}, ErrUnavailable
	}
	if err := c.models.AdmitSelection(cmd.ModelSelection); err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: model selection is not admitted: %w", err)
	}
	draft := schedule.Draft{
		Title:          cmd.Title,
		Instructions:   cmd.Instructions,
		CWD:            cmd.CWD,
		ModelSelection: cmd.ModelSelection,
		Cron:           cmd.Cron,
		Enabled:        cmd.Enabled,
	}
	if err := draft.Validate(); err != nil {
		return schedule.Schedule{}, err
	}
	resolvedCWD, err := c.resolveCWD(draft.CWD)
	if err != nil {
		return schedule.Schedule{}, err
	}
	draft.CWD = resolvedCWD
	created, err := schedule.New(c.newScheduleID(), draft, c.now())
	if err != nil {
		return schedule.Schedule{}, err
	}
	if err := c.store.Insert(ctx, created); err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: create: %w", err)
	}
	c.invalidations.Notify(invalidation.ForSchedules(created.ID()))
	return created, nil
}

// Update applies a patch to an existing schedule, preserving durable identity
// and timestamps while recomputing its next due time.
func (c *Coordinator) Update(ctx context.Context, cmd UpdateCommand) (schedule.Schedule, error) {
	if !c.Available() {
		return schedule.Schedule{}, ErrUnavailable
	}
	if err := schedule.ValidateID(cmd.ID); err != nil {
		return schedule.Schedule{}, err
	}
	if cmd.ExpectedRevision == 0 {
		return schedule.Schedule{}, schedule.ErrRevisionRequired
	}
	existing, err := loadSchedule(ctx, c.store, cmd.ID)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: get %q for update: %w", cmd.ID, err)
	}
	if !cmd.ModelSelection.Empty() {
		selection, selectionErr := cmd.ModelSelection.Apply(existing.ModelSelection())
		if selectionErr != nil {
			return schedule.Schedule{}, selectionErr
		}
		cmd.Patch.Selection = &selection
	}
	if cmd.Patch.Selection != nil {
		if err := c.models.AdmitSelection(*cmd.Patch.Selection); err != nil {
			return schedule.Schedule{}, fmt.Errorf("schedules: model selection is not admitted: %w", err)
		}
	}
	return c.updateExisting(ctx, existing, cmd.Patch, cmd.ExpectedRevision)
}

func (c *Coordinator) updateExisting(
	ctx context.Context,
	existing schedule.Schedule,
	patch schedule.Patch,
	expectedRevision uint64,
) (schedule.Schedule, error) {
	if patch.CWD != nil {
		resolved, err := c.resolveCWD(*patch.CWD)
		if err != nil {
			return schedule.Schedule{}, err
		}
		patch.CWD = &resolved
	}
	updated, err := existing.Edit(patch, expectedRevision, c.now())
	if err != nil {
		return schedule.Schedule{}, err
	}
	if err := c.store.Update(ctx, updated, expectedRevision); err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: update %q: %w", existing.ID(), err)
	}
	c.invalidations.Notify(invalidation.ForSchedules(updated.ID()))
	return updated, nil
}

func loadSchedule(ctx context.Context, store scheduleReader, id string) (schedule.Schedule, error) {
	scheduled, err := store.Get(ctx, id)
	if err != nil {
		return schedule.Schedule{}, err
	}
	if err := scheduled.ValidateStored(); err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: store Get(%q) returned an invalid Schedule: %w", id, err)
	}
	if scheduled.ID() != id {
		return schedule.Schedule{}, fmt.Errorf("schedules: store Get(%q) returned Schedule %q", id, scheduled.ID())
	}
	return scheduled, nil
}

// Delete removes a schedule by id.
func (c *Coordinator) Delete(ctx context.Context, id string) error {
	if !c.Available() {
		return ErrUnavailable
	}
	if err := schedule.ValidateID(id); err != nil {
		return err
	}
	deleted, err := c.store.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("schedules: delete %q: %w", id, err)
	}
	if deleted {
		c.invalidations.Notify(invalidation.ForSchedules(id))
	}
	return nil
}

func (c *Coordinator) resolveCWD(cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	if c.paths == nil {
		return "", errors.Join(workspaceapp.ErrCWDUnavailable, errors.New("schedules: cwd resolver is unavailable"))
	}
	resolved, err := c.paths.ResolveExistingDir(cwd)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %w", workspaceapp.ErrCWDUnavailable, cwd, err)
	}
	return resolved, nil
}
