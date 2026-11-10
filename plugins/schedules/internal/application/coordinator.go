package schedules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/samber/lo"
)

// Only a pre-store refusal proves that a failed command caused no write; store errors retain an unknown outcome.
var ErrWorkspaceUnavailable = errors.New("schedules: workspace is unavailable")

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
	newScheduleID func() string
	now           func() time.Time
}

type CWDResolver interface {
	ResolveExistingDir(path string) (string, error)
}

type Dependencies struct {
	Store         ManagementStore
	Paths         CWDResolver
	NewScheduleID func() string
}

// Untrusted CWD and model spellings must be resolved before they reach the Domain patch.
type Patch struct {
	Title          *string
	Instructions   *string
	CWD            *string
	ModelSelection modelchoice.Patch
	Cron           *string
	TimeZone       *string
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
		{name: "schedule identity factory", value: deps.NewScheduleID},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("schedules: %s is required", required.name)
		}
	}
	return &Coordinator{
		store:         deps.Store,
		paths:         deps.Paths,
		newScheduleID: deps.NewScheduleID,
		now:           time.Now,
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

func (c *Coordinator) PrepareCreate(ctx context.Context, draft schedule.Draft) (ScheduleMutation, error) {
	created, err := c.admit(draft)
	if err != nil {
		return ScheduleMutation{}, refusal{err}
	}
	return ScheduleMutation{state: created, persist: func(ctx context.Context) error {
		return c.store.Insert(ctx, created)
	}}, nil
}

func (c *Coordinator) admit(draft schedule.Draft) (schedule.Schedule, error) {
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

func (c *Coordinator) PrepareUpdate(ctx context.Context, cmd UpdateCommand) (ScheduleMutation, error) {
	if err := schedule.ValidateID(cmd.ID); err != nil {
		return ScheduleMutation{}, err
	}
	if cmd.ExpectedRevision == 0 {
		return ScheduleMutation{}, schedule.ErrRevisionRequired
	}
	existing, err := c.store.Get(ctx, cmd.ID)
	if err != nil {
		return ScheduleMutation{}, fmt.Errorf("schedules: get %q for update: %w", cmd.ID, err)
	}
	patch := schedule.Patch{
		Title: cmd.Patch.Title, Instructions: cmd.Patch.Instructions, CWD: cmd.Patch.CWD,
		Cron: cmd.Patch.Cron, TimeZone: cmd.Patch.TimeZone, Enabled: cmd.Patch.Enabled,
	}
	if !cmd.Patch.ModelSelection.Empty() {
		selection, selectionErr := cmd.Patch.ModelSelection.Apply(existing.ModelSelection())
		if selectionErr != nil {
			return ScheduleMutation{}, selectionErr
		}
		patch.Selection = &selection
	}
	return c.prepareUpdate(existing, patch, cmd.ExpectedRevision)
}

func (c *Coordinator) prepareUpdate(
	existing schedule.Schedule,
	patch schedule.Patch,
	expectedRevision uint64,
) (ScheduleMutation, error) {
	if patch.CWD != nil {
		resolved, err := c.resolveCWD(*patch.CWD)
		if err != nil {
			return ScheduleMutation{}, err
		}
		patch.CWD = &resolved
	}
	updated, err := existing.Edit(patch, expectedRevision, c.now())
	if err != nil {
		return ScheduleMutation{}, err
	}
	replacement, err := schedule.NewReplacement(existing, updated)
	if err != nil {
		return ScheduleMutation{}, fmt.Errorf("schedules: prepare update %q: %w", existing.ID(), err)
	}
	return ScheduleMutation{state: updated, persist: func(ctx context.Context) error {
		return c.store.Update(ctx, replacement)
	}}, nil
}

func (c *Coordinator) PrepareDelete(ctx context.Context, id string) (func(context.Context) error, error) {
	if err := schedule.ValidateID(id); err != nil {
		return nil, refusal{err}
	}
	if _, err := c.store.Get(ctx, id); err != nil {
		if errors.Is(err, schedule.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return func(ctx context.Context) error {
		deleted, err := c.store.Delete(ctx, id)
		if err != nil {
			return err
		}
		if !deleted {
			return errors.New("schedules: decided deletion lost its original schedule")
		}
		return nil
	}, nil
}

func (c *Coordinator) resolveCWD(cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	resolved, err := c.paths.ResolveExistingDir(cwd)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %w", ErrWorkspaceUnavailable, cwd, err)
	}
	return resolved, nil
}
