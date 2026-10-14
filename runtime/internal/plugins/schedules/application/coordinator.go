package schedules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	modelchoice "github.com/Tangerg/flame/runtime/model"
)

// Only a pre-store refusal proves that a failed command caused no write; store errors retain an unknown outcome.
var ErrRefused = errors.New("schedules: refused before any change")

type refusal struct{ cause error }

func (r refusal) Error() string      { return r.cause.Error() }
func (r refusal) Unwrap() error      { return r.cause }
func (refusal) Is(target error) bool { return target == ErrRefused }

// Management writes exclude cursor claims and acceptance transitions, which enter their own store boundary.
type ManagementStore interface {
	ListPage(ctx context.Context, afterCreatedAt time.Time, afterID string, limit int) ([]schedule.Schedule, error)
	Get(ctx context.Context, id string) (schedule.Schedule, error)
	Insert(ctx context.Context, scheduled schedule.Schedule) error
	Update(ctx context.Context, replacement schedule.Replacement) error
	Delete(ctx context.Context, id string) (bool, error)
}

type Coordinator struct {
	store         ManagementStore
	paths         CWDResolver
	models        ModelAdmitter
	newScheduleID func() string
	now           func() time.Time
	invalidations invalidation.Publish
}

type CWDResolver interface {
	ResolveExistingDir(path string) (string, error)
}

type ModelAdmitter interface {
	AdmitSelection(selection modelchoice.Selection) error
}

type Dependencies struct {
	Store         ManagementStore
	Paths         CWDResolver
	Models        ModelAdmitter
	NewScheduleID func() string
	Invalidations invalidation.Publish
}

// Untrusted CWD and model spellings must be resolved before they reach the Domain patch.
type Patch struct {
	Title          *string
	Instructions   *string
	CWD            *string
	ModelSelection modelchoice.Patch
	Cron           *string
	Enabled        *bool
}

type UpdateCommand struct {
	ID               string
	Patch            Patch
	ExpectedRevision uint64
}

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
		if dependency.Missing(required.value) {
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

const listPageNamespace = "schedules-created-ms"

const listPageLimit = 100

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
		afterCreatedAtMillis, parseErr := strconv.ParseInt(anchor[0], 10, 64)
		if parseErr != nil {
			return pagination.Page[schedule.Schedule]{}, pagination.ErrInvalidCursor
		}
		afterCreatedAt = time.UnixMilli(afterCreatedAtMillis).UTC()
		afterID = anchor[1]
		if err := schedule.ValidateID(afterID); err != nil {
			return pagination.Page[schedule.Schedule]{}, pagination.ErrInvalidCursor
		}
	}
	size, err := limit.Resolve(listPageLimit)
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	rows, err := c.store.ListPage(ctx, afterCreatedAt, afterID, size+1)
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	return pagination.PageOf(rows, size, listPageNamespace, nil, func(scheduled schedule.Schedule) []string {
		return []string{strconv.FormatInt(scheduled.CreatedAt().UnixMilli(), 10), scheduled.ID()}
	})
}

func (c *Coordinator) Create(ctx context.Context, draft schedule.Draft) (schedule.Schedule, error) {
	created, err := c.admit(draft)
	if err != nil {
		return schedule.Schedule{}, refusal{err}
	}
	if err := c.store.Insert(ctx, created); err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: create: %w", err)
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.PluginData})
	return created, nil
}

func (c *Coordinator) admit(draft schedule.Draft) (schedule.Schedule, error) {
	if err := c.models.AdmitSelection(draft.ModelSelection); err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: model selection is not admitted: %w", err)
	}
	if err := draft.Validate(); err != nil {
		return schedule.Schedule{}, err
	}
	resolvedCWD, err := c.resolveCWD(draft.CWD)
	if err != nil {
		return schedule.Schedule{}, err
	}
	draft.CWD = resolvedCWD
	return schedule.New(c.newScheduleID(), draft, c.now())
}

func (c *Coordinator) Update(ctx context.Context, cmd UpdateCommand) (schedule.Schedule, error) {
	if err := schedule.ValidateID(cmd.ID); err != nil {
		return schedule.Schedule{}, err
	}
	if cmd.ExpectedRevision == 0 {
		return schedule.Schedule{}, schedule.ErrRevisionRequired
	}
	existing, err := c.store.Get(ctx, cmd.ID)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: get %q for update: %w", cmd.ID, err)
	}
	patch := schedule.Patch{
		Title: cmd.Patch.Title, Instructions: cmd.Patch.Instructions, CWD: cmd.Patch.CWD,
		Cron: cmd.Patch.Cron, Enabled: cmd.Patch.Enabled,
	}
	if !cmd.Patch.ModelSelection.Empty() {
		selection, selectionErr := cmd.Patch.ModelSelection.Apply(existing.ModelSelection())
		if selectionErr != nil {
			return schedule.Schedule{}, selectionErr
		}
		patch.Selection = &selection
	}
	if patch.Selection != nil {
		if err := c.models.AdmitSelection(*patch.Selection); err != nil {
			return schedule.Schedule{}, fmt.Errorf("schedules: model selection is not admitted: %w", err)
		}
	}
	return c.updateExisting(ctx, existing, patch, cmd.ExpectedRevision)
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
	replacement, err := schedule.NewReplacement(existing, updated)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: prepare update %q: %w", existing.ID(), err)
	}
	if err := c.store.Update(ctx, replacement); err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: update %q: %w", existing.ID(), err)
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.PluginData})
	return updated, nil
}

func (c *Coordinator) Delete(ctx context.Context, id string) error {
	if err := schedule.ValidateID(id); err != nil {
		return refusal{err}
	}
	deleted, err := c.store.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("schedules: delete %q: %w", id, err)
	}
	if deleted {
		c.invalidations.Notify(invalidation.Notice{Resource: invalidation.PluginData})
	}
	return nil
}

func (c *Coordinator) resolveCWD(cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	resolved, err := c.paths.ResolveExistingDir(cwd)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %w", workspaceapp.ErrCWDUnavailable, cwd, err)
	}
	return resolved, nil
}
