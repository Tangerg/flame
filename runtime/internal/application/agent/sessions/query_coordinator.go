package sessions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/samber/lo"
)

const (
	itemPageNamespace      = "items"
	runPageNamespace       = "runs"
	interruptPageNamespace = "interrupts"
)

const (
	itemPageLimit            = 200
	runPageLimit             = 100
	interruptPageLimit       = 100
	modelInvocationPageLimit = 100
	trajectoryPageLimit      = 100
)

type queryTranscriptReader interface {
	Item(ctx context.Context, itemID identity.ItemID) (transcript.Item, bool, error)
	PageSessionItems(ctx context.Context, sessionID identity.SessionID, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error)
	PageRunItems(ctx context.Context, runID identity.RunID, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error)
	PageRunTreeItems(ctx context.Context, runID identity.RunID, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error)
}

type queryPlanReader interface {
	State(ctx context.Context, sessionID identity.SessionID) (plan.Current, error)
}

type querySessionReader interface {
	Exists(ctx context.Context, sessionID identity.SessionID) (bool, error)
}

type QueryInterruptReader interface {
	ListPage(ctx context.Context, sessionID identity.SessionID, rootRunID identity.RunID, after *run.PageAnchor, limit int) ([]runs.Pending, error)
}

type queryRunReader interface {
	Run(ctx context.Context, runID identity.RunID) (run.Run, bool, error)
	RunsWithAncestors(ctx context.Context, runIDs []identity.RunID) ([]run.Run, error)
	PageRuns(ctx context.Context, sessionID identity.SessionID, statuses []run.State, includeDescendants bool, before *run.PageAnchor, limit int) ([]run.Run, error)
}

type QueryCoordinator struct {
	readSnapshot     func(context.Context, func(context.Context) error) error
	trajectory       queryTrajectoryReader
	modelInvocations queryModelInvocationReader
	transcript       queryTranscriptReader
	interrupts       QueryInterruptReader
	runs             queryRunReader
	sessions         querySessionReader
	plan             queryPlanReader
}

type QueryDependencies struct {
	ReadSnapshot     func(context.Context, func(context.Context) error) error
	Trajectory       queryTrajectoryReader
	ModelInvocations queryModelInvocationReader
	Transcript       queryTranscriptReader
	Interrupts       QueryInterruptReader
	Runs             queryRunReader
	Sessions         querySessionReader
	Plan             queryPlanReader
}

func NewQueryCoordinator(deps QueryDependencies) (*QueryCoordinator, error) {
	required := []struct {
		name  string
		value any
	}{
		{"transcript reader", deps.Transcript},
		{"interrupt reader", deps.Interrupts},
		{"Run reader", deps.Runs},
		{"session reader", deps.Sessions},
		{"Plan reader", deps.Plan},
		{"model invocation reader", deps.ModelInvocations},
		{"trajectory reader", deps.Trajectory},
		{"snapshot read", deps.ReadSnapshot},
	}
	for _, required := range required {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("sessions: query %s is required", required.name)
		}
	}
	return &QueryCoordinator{
		readSnapshot:     deps.ReadSnapshot,
		trajectory:       deps.Trajectory,
		transcript:       deps.Transcript,
		modelInvocations: deps.ModelInvocations,
		interrupts:       deps.Interrupts,
		runs:             deps.Runs,
		sessions:         deps.Sessions,
		plan:             deps.Plan,
	}, nil
}

type ItemPage struct {
	Items      []transcript.Item
	NextCursor string
	Runs       []run.Run
}

type itemScopeKind uint8

const (
	sessionItemScope itemScopeKind = iota + 1
	runItemScope
)

var errInvalidItemScope = errors.New("sessions: item query scope is invalid")

type ItemScope struct {
	kind               itemScopeKind
	sessionID          identity.SessionID
	runID              identity.RunID
	includeDescendants bool
}

func (i ItemScope) RunID() (identity.RunID, bool) { return i.runID, i.kind == runItemScope }

func Items(sessionID identity.SessionID) ItemScope {
	return ItemScope{kind: sessionItemScope, sessionID: sessionID}
}

func RunItems(runID identity.RunID) ItemScope {
	return ItemScope{kind: runItemScope, runID: runID}
}

func RunTreeItems(runID identity.RunID) ItemScope {
	return ItemScope{
		kind:               runItemScope,
		runID:              runID,
		includeDescendants: true,
	}
}

func (c *QueryCoordinator) ListItemPage(ctx context.Context, scope ItemScope, order transcript.SequenceOrder, cursor string, limit pagination.RequestedLimit) (ItemPage, error) {
	if err := order.Validate(); err != nil {
		return ItemPage{}, err
	}
	filters, err := scope.cursorFilters(order)
	if err != nil {
		return ItemPage{}, err
	}
	anchor, err := pagination.Decode(cursor, itemPageNamespace, filters)
	if err != nil {
		return ItemPage{}, err
	}
	fromSequence, err := sequenceAnchor(anchor)
	if err != nil {
		return ItemPage{}, err
	}
	size, err := limit.Resolve(itemPageLimit)
	if err != nil {
		return ItemPage{}, err
	}
	var result ItemPage
	err = c.readSnapshot(ctx, func(ctx context.Context) error {
		if requireScopeErr := c.requireScope(ctx, scope); requireScopeErr != nil {
			return requireScopeErr
		}

		sequenced, err := c.readScope(ctx, scope, order, fromSequence, size+1)
		if err != nil {
			return err
		}
		page, err := pagination.PageOf(sequenced, size, itemPageNamespace, filters,
			func(entry transcript.SequencedItem) []string {
				return []string{strconv.FormatInt(entry.Sequence, 10)}
			})
		if err != nil {
			return err
		}

		items := make([]transcript.Item, 0, len(page.Rows))
		for _, entry := range page.Rows {
			items = append(items, entry.Item)
		}
		runs, err := c.runs.RunsWithAncestors(ctx, referencedRuns(items))
		if err != nil {
			return err
		}
		result = ItemPage{Items: items, NextCursor: page.NextCursor, Runs: runs}
		return nil
	})
	if err != nil {
		return ItemPage{}, err
	}
	return result, nil
}

func (c *QueryCoordinator) PlanState(ctx context.Context, sessionID identity.SessionID) (plan.Current, error) {
	if parseErr := sessionID.Validate(); parseErr != nil {
		return plan.Current{}, fmt.Errorf("sessions: query Plan: %w", parseErr)
	}
	found, err := c.sessions.Exists(ctx, sessionID)
	if err != nil {
		return plan.Current{}, err
	}
	if !found {
		return plan.Current{}, session.ErrNotFound
	}
	return c.plan.State(ctx, sessionID)
}

func (c *QueryCoordinator) requireScope(ctx context.Context, scope ItemScope) error {
	switch scope.kind {
	case runItemScope:
		_, found, err := c.runs.Run(ctx, scope.runID)
		if err != nil {
			return err
		}
		if !found {
			return transcript.ErrRunNotFound
		}
		return nil
	case sessionItemScope:
		found, err := c.sessions.Exists(ctx, scope.sessionID)
		if err != nil {
			return err
		}
		if !found {
			return session.ErrNotFound
		}
		return nil
	default:
		return errInvalidItemScope
	}
}

func (c *QueryCoordinator) readScope(ctx context.Context, scope ItemScope, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	switch scope.kind {
	case runItemScope:
		if scope.includeDescendants {
			return c.transcript.PageRunTreeItems(ctx, scope.runID, order, fromSequence, limit)
		}
		return c.transcript.PageRunItems(ctx, scope.runID, order, fromSequence, limit)
	case sessionItemScope:
		return c.transcript.PageSessionItems(ctx, scope.sessionID, order, fromSequence, limit)
	default:
		return nil, errInvalidItemScope
	}
}

func (i ItemScope) cursorFilters(order transcript.SequenceOrder) ([]string, error) {
	switch i.kind {
	case sessionItemScope:
		if err := i.sessionID.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %w", errInvalidItemScope, err)
		}
		return []string{i.sessionID.String(), "", strconv.FormatBool(false), order.String()}, nil
	case runItemScope:
		if err := i.runID.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %w", errInvalidItemScope, err)
		}
		return []string{"", i.runID.String(), strconv.FormatBool(i.includeDescendants), order.String()}, nil
	default:
		return nil, errInvalidItemScope
	}
}

func referencedRuns(items []transcript.Item) []identity.RunID {
	var out []identity.RunID
	for _, item := range items {
		if !item.RunID().IsZero() && !slices.Contains(out, item.RunID()) {
			out = append(out, item.RunID())
		}
	}
	return out
}

func sequenceAnchor(anchor []string) (int64, error) {
	if len(anchor) == 0 {
		return 0, nil
	}
	sequence, err := strconv.ParseInt(anchor[0], 10, 64)
	if err != nil || len(anchor) != 1 || sequence <= 0 {
		return 0, pagination.ErrInvalidCursor
	}
	return sequence, nil
}

func (c *QueryCoordinator) Run(ctx context.Context, runID identity.RunID) (run.Run, bool, error) {
	if runID.IsZero() {
		return run.Run{}, false, nil
	}
	if err := runID.Validate(); err != nil {
		return run.Run{}, false, fmt.Errorf("sessions: query Run: %w", err)
	}
	return c.runs.Run(ctx, runID)
}

type RunPageFilter struct {
	SessionID          identity.SessionID
	Statuses           []run.State
	IncludeDescendants bool
}

func (c *QueryCoordinator) ListRunPage(ctx context.Context, filter RunPageFilter, cursor string, limit pagination.RequestedLimit) (pagination.Page[run.Run], error) {
	filter.Statuses = normalizeStatuses(filter.Statuses)
	if err := filter.validate(); err != nil {
		return pagination.Page[run.Run]{}, err
	}
	filters := []string{
		filter.SessionID.String(),
		statusFilter(filter.Statuses),
		strconv.FormatBool(filter.IncludeDescendants),
	}
	before, err := runPageAnchor(cursor, runPageNamespace, filters)
	if err != nil {
		return pagination.Page[run.Run]{}, err
	}
	size, err := limit.Resolve(runPageLimit)
	if err != nil {
		return pagination.Page[run.Run]{}, err
	}
	rows, err := c.runs.PageRuns(
		ctx,
		filter.SessionID,
		filter.Statuses,
		filter.IncludeDescendants,
		before,
		size+1,
	)
	if err != nil {
		return pagination.Page[run.Run]{}, err
	}
	return pagination.PageOf(rows, size, runPageNamespace, filters, func(run run.Run) []string {
		return []string{strconv.FormatInt(run.CreatedAt().UnixNano(), 10), run.ID().String()}
	})
}

func (f RunPageFilter) validate() error {
	for _, status := range f.Statuses {
		if !status.Valid() {
			return fmt.Errorf("sessions: query Runs page has invalid status %q", status)
		}
	}
	return nil
}

func normalizeStatuses(statuses []run.State) []run.State {
	if len(statuses) == 0 {
		return nil
	}
	normalized := slices.Clone(statuses)
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func statusFilter(statuses []run.State) string {
	names := make([]string, 0, len(statuses))
	for _, status := range statuses {
		names = append(names, string(status))
	}
	return strings.Join(names, ",")
}

func (c *QueryCoordinator) answerableInterruptSet(ctx context.Context, pending runs.Pending, caller run.Capabilities) (InterruptSet, error) {
	root, found, err := c.runs.Run(ctx, pending.RootRunID)
	if err != nil {
		return InterruptSet{}, err
	}
	if !found {
		return InterruptSet{}, fmt.Errorf("sessions: pending set %q has no root Run", pending.RootRunID)
	}
	if gap := root.Capabilities().MissingFrom(caller); !gap.IsEmpty() {
		return InterruptSet{}, fmt.Errorf("sessions: run %q: %w", pending.RootRunID, &run.InsufficientCapabilitiesError{Missing: gap})
	}
	set, err := c.interruptSet(ctx, pending)
	if err != nil {
		return InterruptSet{}, err
	}
	set.SessionID = root.SessionID()
	return set, nil
}

func (c *QueryCoordinator) ListPendingInterruptPage(ctx context.Context, sessionID identity.SessionID, rootRunID identity.RunID, caller run.Capabilities, cursor string, limit pagination.RequestedLimit) (pagination.Page[InterruptSet], error) {
	caller = caller.Clone()
	if err := caller.Validate(); err != nil {
		return pagination.Page[InterruptSet]{}, fmt.Errorf("sessions: query interrupts page caller capabilities: %w", err)
	}
	filters := []string{sessionID.String(), rootRunID.String()}
	after, err := runPageAnchor(cursor, interruptPageNamespace, filters)
	if err != nil {
		return pagination.Page[InterruptSet]{}, err
	}
	size, err := limit.Resolve(interruptPageLimit)
	if err != nil {
		return pagination.Page[InterruptSet]{}, err
	}
	var result pagination.Page[InterruptSet]
	err = c.readSnapshot(ctx, func(ctx context.Context) error {
		if requireRootErr := c.requireRoot(ctx, rootRunID); requireRootErr != nil {
			return requireRootErr
		}
		rows, err := c.interrupts.ListPage(ctx, sessionID, rootRunID, after, size+1)
		if err != nil {
			return err
		}
		page, err := pagination.PageOf(rows, size, interruptPageNamespace, filters, func(pending runs.Pending) []string {
			return []string{strconv.FormatInt(pending.CreatedAt.UnixNano(), 10), pending.RootRunID.String()}
		})
		if err != nil {
			return err
		}
		sets := make([]InterruptSet, 0, len(page.Rows))
		for _, pending := range page.Rows {
			set, err := c.answerableInterruptSet(ctx, pending, caller)
			if err != nil {
				return err
			}
			sets = append(sets, set)
		}
		result = pagination.Page[InterruptSet]{Rows: sets, NextCursor: page.NextCursor}
		return nil
	})
	if err != nil {
		return pagination.Page[InterruptSet]{}, err
	}
	return result, nil
}

func (c *QueryCoordinator) interruptSet(ctx context.Context, pending runs.Pending) (InterruptSet, error) {
	itemsByID := make(map[identity.ItemID]transcript.Item, len(pending.Interrupts))
	for _, open := range pending.Interrupts {
		item, found, err := c.transcript.Item(ctx, open.ItemID)
		if err != nil {
			return InterruptSet{}, err
		}
		if found {
			itemsByID[open.ItemID] = item
		}
	}
	interrupts, err := pending.ProjectInterrupts(itemsByID)
	if err != nil {
		return InterruptSet{}, fmt.Errorf("sessions: pending set %q: %w", pending.RootRunID, err)
	}
	return InterruptSet{Pending: pending, Interrupts: interrupts}, nil
}

func (c *QueryCoordinator) requireRoot(ctx context.Context, runID identity.RunID) error {
	if runID.IsZero() {
		return nil
	}
	if err := runID.Validate(); err != nil {
		return fmt.Errorf("sessions: query root Run: %w", err)
	}
	run, found, err := c.runs.Run(ctx, runID)
	if err != nil || !found {
		return err
	}
	if run.Lineage().IsChild() {
		return fmt.Errorf("%w: run %q belongs to the tree rooted elsewhere", transcript.ErrNotRoot, runID)
	}
	return nil
}

// The Run identity makes the order total: two rows can share a nanosecond.
func runPageAnchor(cursor, method string, filters []string) (*run.PageAnchor, error) {
	anchor, err := pagination.Decode(cursor, method, filters)
	if err != nil {
		return nil, err
	}
	if len(anchor) == 0 {
		return nil, nil
	}
	if len(anchor) != 2 {
		return nil, pagination.ErrInvalidCursor
	}
	stamp, err := strconv.ParseInt(anchor[0], 10, 64)
	if err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	runID, err := identity.ParseRun(anchor[1])
	if err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	position, err := run.NewPageAnchor(time.Unix(0, stamp), runID)
	if err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	return &position, nil
}
