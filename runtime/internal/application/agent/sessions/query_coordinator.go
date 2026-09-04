package sessions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// Each namespace binds a cursor to one application read so another read cannot
// continue it. Names are semantic and deliberately independent of transport
// method names; cursors are opaque application values.
const (
	itemPageNamespace      = "items"
	runPageNamespace       = "runs"
	interruptPageNamespace = "interrupts"
)

// Page ceilings, per read. A client asking for more gets this many and a cursor.
const (
	itemPageLimit      = 200
	runPageLimit       = 100
	interruptPageLimit = 100
)

// QueryTranscriptReader is the coordinator's view of the durable item history. Items
// arrive one bounded page at a time, seeking from the previous page's position in
// the direction asked for.
//
// The two scopes are two methods rather than one method with a nullable subject:
// "exactly one of these is set" is a contract nothing checks, and each of these
// reads one thing.
type QueryTranscriptReader interface {
	PageSessionItems(ctx context.Context, sessionID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error)
	PageRunItems(ctx context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error)
	PageRunTreeItems(ctx context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error)
}

// QueryPlanReader is the coordinator's view of the Plan projection: the whole
// state, because what identifies one replacement from the next is its revision.
type QueryPlanReader interface {
	State(ctx context.Context, sessionID string) (plan.Current, error)
}

// QuerySessionReader answers only whether a session exists. The item read needs that
// much and no more: a scope naming no session is a refusal, and an empty page would
// tell the client its session is empty instead.
type QuerySessionReader interface {
	Exists(ctx context.Context, sessionID string) (bool, error)
}

// QueryInterruptReader is the coordinator's view of the open-interrupt registry. Both
// filters are optional and independent: empty means "every", and given together they
// both apply.
type QueryInterruptReader interface {
	ListPage(ctx context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRootRunID string, limit int) ([]runs.Pending, error)
}

// QueryRunReader is the coordinator's view of the durable Run record: one Run by id,
// the ancestor closure of a named set, and a browsable page of Runs. The closure
// threads a page of items onto a connected tree without loading unrelated Runs
// from a long session.
type QueryRunReader interface {
	Run(ctx context.Context, runID string) (run.Run, bool, error)
	RunsWithAncestors(ctx context.Context, runIDs []string) ([]run.Run, error)
	PageRuns(ctx context.Context, sessionID string, statuses []run.Status, includeDescendants bool, beforeCreatedAt int64, beforeRunID string, limit int) ([]run.Run, error)
}

// QueryCoordinator serves the session read projections. Stateless beyond its store
// collaborators; safe to share.
type QueryCoordinator struct {
	transcript QueryTranscriptReader
	interrupts QueryInterruptReader
	runs       QueryRunReader
	sessions   QuerySessionReader
	plan       QueryPlanReader
}

// QueryDependencies is the collaborator set [NewQueryCoordinator] wires into a QueryCoordinator.
type QueryDependencies struct {
	Transcript QueryTranscriptReader
	Interrupts QueryInterruptReader
	Runs       QueryRunReader
	Sessions   QuerySessionReader
	Plan       QueryPlanReader
}

// NewQueryCoordinator returns a complete query coordinator over deps.
func NewQueryCoordinator(deps QueryDependencies) (*QueryCoordinator, error) {
	required := []struct {
		name  string
		value any
	}{
		{"transcript reader", deps.Transcript},
		{"interrupt reader", deps.Interrupts},
		{"Run reader", deps.Runs},
		{"session reader", deps.Sessions},
	}
	for _, dependency := range required {
		if nilDependency(dependency.value) {
			return nil, fmt.Errorf("sessions: query %s is required", dependency.name)
		}
	}
	if deps.Plan != nil && nilDependency(deps.Plan) {
		return nil, errors.New("sessions: optional query Plan reader must not be typed nil")
	}
	return &QueryCoordinator{
		transcript: deps.Transcript,
		interrupts: deps.Interrupts,
		runs:       deps.Runs,
		sessions:   deps.Sessions,
		plan:       deps.Plan,
	}, nil
}

// ItemPage is one page of a session's history, with the run tree needed to thread
// the items on it.
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

// ItemScope is the closed application choice of a whole session timeline, one
// Run's own items, or that Run's complete subtree. Its fields stay private so
// callers cannot construct a scope that names both a Session and a Run, or ask a
// Session for descendants it already contains.
type ItemScope struct {
	kind               itemScopeKind
	subjectID          string
	includeDescendants bool
}

// Items scopes a page to a session's whole timeline.
func Items(sessionID string) ItemScope {
	return ItemScope{kind: sessionItemScope, subjectID: sessionID}
}

// RunItems scopes a page to one Run's own items. The Run's session is resolved from
// the Run, so no caller has to supply both and risk supplying two different ones.
func RunItems(runID string) ItemScope {
	return ItemScope{kind: runItemScope, subjectID: runID}
}

// RunTreeItems scopes a page to one Run and all of its descendants. The subject
// may itself be a child; ancestors are not part of the item scope.
func RunTreeItems(runID string) ItemScope {
	return ItemScope{
		kind:               runItemScope,
		subjectID:          runID,
		includeDescendants: true,
	}
}

// ListItemPage returns one page of durable history within scope, in the direction
// order names, continuing from cursor. A page is bounded in the query: the previous
// page's position is the seek anchor, so serving the tail of a long session costs a
// page, not the whole timeline.
//
// A scope naming nothing that exists is refused with [session.ErrNotFound] or
// [transcript.ErrRunNotFound]. An empty page would be a worse answer to a wrong id
// than an error is: it says the session or run is empty, which is a fact about
// something that does not exist.
//
// An unusable cursor is refused rather than reinterpreted — see
// [pagination.ErrInvalidCursor]. Silently restarting from the top would look like a
// page of duplicates to a client that had already read them.
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
	if requireScopeErr := c.requireScope(ctx, scope); requireScopeErr != nil {
		return ItemPage{}, requireScopeErr
	}

	// One row past the page: having it is how "there is more" is known without a
	// second count, and it is dropped before the page is returned.
	sequenced, err := c.readScope(ctx, scope, order, fromSequence, size+1)
	if err != nil {
		return ItemPage{}, err
	}
	if err := validateItemRows(sequenced, scope, order, fromSequence, size+1); err != nil {
		return ItemPage{}, err
	}
	page, err := pagination.PageOf(sequenced, size, itemPageNamespace, filters,
		func(entry transcript.SequencedItem) []string {
			return []string{strconv.FormatInt(entry.Sequence, 10)}
		})
	if err != nil {
		return ItemPage{}, err
	}

	items := make([]transcript.Item, 0, len(page.Rows))
	for _, entry := range page.Rows {
		items = append(items, entry.Item)
	}
	runs, err := c.runs.RunsWithAncestors(ctx, referencedRuns(items))
	if err != nil {
		return ItemPage{}, err
	}
	if err := validateItemRunClosure(scope, items, runs); err != nil {
		return ItemPage{}, err
	}
	runs = slices.Clone(runs)
	return ItemPage{Items: items, NextCursor: page.NextCursor, Runs: runs}, nil
}

func validateItemRows(rows []transcript.SequencedItem, scope ItemScope, order transcript.SequenceOrder, anchor int64, maximum int) error {
	if len(rows) > maximum {
		return fmt.Errorf("sessions: transcript store returned %d rows, maximum %d", len(rows), maximum)
	}
	seenItems := make(map[string]struct{}, len(rows))
	for index, entry := range rows {
		if entry.Sequence <= 0 {
			return fmt.Errorf("sessions: transcript store row %d has invalid sequence %d", index+1, entry.Sequence)
		}
		if err := entry.Item.Validate(); err != nil {
			return fmt.Errorf("sessions: transcript store row %d is invalid: %w", index+1, err)
		}
		if err := scope.validateDirectItem(entry.Item); err != nil {
			return err
		}
		if _, duplicate := seenItems[entry.Item.ID()]; duplicate {
			return fmt.Errorf("sessions: transcript page repeats Item %q", entry.Item.ID())
		}
		seenItems[entry.Item.ID()] = struct{}{}
		if anchor > 0 && !sequenceFollows(entry.Sequence, anchor, order) {
			return fmt.Errorf("sessions: transcript Item %q does not follow the page cursor", entry.Item.ID())
		}
		if index > 0 && !sequenceFollows(entry.Sequence, rows[index-1].Sequence, order) {
			return fmt.Errorf("sessions: transcript Item %q is out of order after %q", entry.Item.ID(), rows[index-1].Item.ID())
		}
	}
	return nil
}

func (i ItemScope) validateDirectItem(item transcript.Item) error {
	switch i.kind {
	case sessionItemScope:
		if item.SessionID() != i.subjectID {
			return fmt.Errorf("sessions: transcript Item %q does not belong to Session %q", item.ID(), i.subjectID)
		}
	case runItemScope:
		if !i.includeDescendants && item.RunID() != i.subjectID {
			return fmt.Errorf("sessions: transcript Item %q does not belong to Run %q", item.ID(), i.subjectID)
		}
	default:
		return errInvalidItemScope
	}
	return nil
}

func sequenceFollows(sequence, previous int64, order transcript.SequenceOrder) bool {
	if order == transcript.NewestFirst {
		return sequence < previous
	}
	return sequence > previous
}

func validateItemRunClosure(scope ItemScope, items []transcript.Item, values []run.Run) error {
	runsByID := make(map[string]run.Run, len(values))
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("sessions: Item page Run[%d] is invalid: %w", index, err)
		}
		if _, duplicate := runsByID[value.ID()]; duplicate {
			return fmt.Errorf("sessions: Item page repeats Run %q", value.ID())
		}
		runsByID[value.ID()] = value
	}
	if err := validateItemRunTrees(runsByID); err != nil {
		return err
	}

	required := make(map[string]struct{}, len(values))
	for _, item := range items {
		owner, found := runsByID[item.RunID()]
		if !found {
			return fmt.Errorf("sessions: Item page omits Run %q referenced by Item %q", item.RunID(), item.ID())
		}
		if owner.SessionID() != item.SessionID() {
			return fmt.Errorf("sessions: Item %q and Run %q belong to different Sessions", item.ID(), owner.ID())
		}
		inRequestedTree := scope.kind == sessionItemScope
		current := owner
		for {
			if current.SessionID() != item.SessionID() {
				return fmt.Errorf("sessions: Run %q crosses the Session boundary for Item %q", current.ID(), item.ID())
			}
			required[current.ID()] = struct{}{}
			if current.ID() == scope.subjectID {
				inRequestedTree = true
			}
			if current.Lineage().IsRoot() {
				break
			}
			parentID := current.Lineage().ParentRunID
			parent, exists := runsByID[parentID]
			if !exists {
				return fmt.Errorf("sessions: Item page omits parent Run %q of %q", parentID, current.ID())
			}
			current = parent
		}
		if !inRequestedTree {
			return fmt.Errorf("sessions: Item %q belongs outside Run subtree %q", item.ID(), scope.subjectID)
		}
	}
	if len(required) != len(runsByID) {
		return errors.New("sessions: Item page contains a Run outside its referenced ancestor closure")
	}
	return nil
}

func validateItemRunTrees(values map[string]run.Run) error {
	byRoot := make(map[string][]run.TreeMember)
	for _, value := range values {
		rootID := value.Lineage().TreeRootID(value.ID())
		byRoot[rootID] = append(byRoot[rootID], run.TreeMember{RunID: value.ID(), Lineage: value.Lineage()})
	}
	for rootID, members := range byRoot {
		if _, err := run.NewTree(rootID, members); err != nil {
			return fmt.Errorf("sessions: Item page Run closure: %w", err)
		}
	}
	return nil
}

// PlanState returns a session's optional Plan projection. An existing Session
// with no committed replacement returns an explicit unwritten Current; only an
// unknown Session is [session.ErrNotFound].
func (c *QueryCoordinator) PlanState(ctx context.Context, sessionID string) (plan.Current, error) {
	if _, parseErr := resourceid.ParseSession(sessionID); parseErr != nil {
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

// requireScope refuses a scope whose subject does not exist. It runs after the
// cursor and limit are validated: a malformed request is malformed whether or not
// its subject exists, and answering "no such session" to a request that was never
// answerable would send the caller looking in the wrong place.
func (c *QueryCoordinator) requireScope(ctx context.Context, scope ItemScope) error {
	switch scope.kind {
	case runItemScope:
		_, found, err := c.runs.Run(ctx, scope.subjectID)
		if err != nil {
			return err
		}
		if !found {
			return transcript.ErrRunNotFound
		}
		return nil
	case sessionItemScope:
		found, err := c.sessions.Exists(ctx, scope.subjectID)
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
			return c.transcript.PageRunTreeItems(ctx, scope.subjectID, order, fromSequence, limit)
		}
		return c.transcript.PageRunItems(ctx, scope.subjectID, order, fromSequence, limit)
	case sessionItemScope:
		return c.transcript.PageSessionItems(ctx, scope.subjectID, order, fromSequence, limit)
	default:
		return nil, errInvalidItemScope
	}
}

func (i ItemScope) cursorFilters(order transcript.SequenceOrder) ([]string, error) {
	switch i.kind {
	case sessionItemScope:
		if _, err := resourceid.ParseSession(i.subjectID); err != nil {
			return nil, fmt.Errorf("%w: %v", errInvalidItemScope, err)
		}
		return []string{i.subjectID, "", strconv.FormatBool(false), order.String()}, nil
	case runItemScope:
		if _, err := resourceid.ParseRun(i.subjectID); err != nil {
			return nil, fmt.Errorf("%w: %v", errInvalidItemScope, err)
		}
		return []string{"", i.subjectID, strconv.FormatBool(i.includeDescendants), order.String()}, nil
	default:
		return nil, errInvalidItemScope
	}
}

// referencedRuns is the distinct Runs this page's items belong to, in first-seen
// order. It is what the page carries instead of the session's Run list: the client
// merges these across pages, so what it can thread is exactly what it has read.
func referencedRuns(items []transcript.Item) []string {
	var out []string
	for _, item := range items {
		if item.RunID() != "" && !slices.Contains(out, item.RunID()) {
			out = append(out, item.RunID())
		}
	}
	return out
}

// sequenceAnchor reads a decoded cursor's sort position. A token whose key is not
// a sequence was not minted by this read, whatever else about it matched.
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

// Run returns one Run by id, reporting false when no Run has that id. It reads
// the durable record, so a Run this process never streamed — parked, finished, or
// admitted before a restart — answers the same as one it is streaming now.
func (c *QueryCoordinator) Run(ctx context.Context, runID string) (run.Run, bool, error) {
	if runID == "" {
		return run.Run{}, false, nil
	}
	if _, err := resourceid.ParseRun(runID); err != nil {
		return run.Run{}, false, fmt.Errorf("sessions: query Run: %w", err)
	}
	return c.runs.Run(ctx, runID)
}

// RunPageFilter is every collection-defining input to [QueryCoordinator.ListRunPage].
// Keeping it named makes includeDescendants part of the query rather than an
// easily misplaced positional boolean, and gives the cursor one explicit filter
// identity to bind.
type RunPageFilter struct {
	SessionID          string
	Statuses           []run.Status
	IncludeDescendants bool
}

// ListRunPage returns one page of Runs matching filter, newest admission first,
// continuing after cursor. An empty SessionID pages across every session, empty
// Statuses match every lifecycle position, and IncludeDescendants false restricts
// the page to roots. A finished Run remains part of history.
//
// It reads the durable admission record rather than a live in-process registry:
// the registry only knows the segments THIS process is streaming, so it answers a
// different question, and answers it differently after a restart.
//
// The cursor is bound to the normalized filter, not just to the method: continuing
// a page under a different session or status set would seek into a collection the
// anchor was never a position in.
func (c *QueryCoordinator) ListRunPage(ctx context.Context, filter RunPageFilter, cursor string, limit pagination.RequestedLimit) (pagination.Page[run.Run], error) {
	if err := filter.validate(); err != nil {
		return pagination.Page[run.Run]{}, err
	}
	filter.Statuses = normalizeStatuses(filter.Statuses)
	filters := []string{
		filter.SessionID,
		statusFilter(filter.Statuses),
		strconv.FormatBool(filter.IncludeDescendants),
	}
	beforeCreatedAt, beforeID, err := timeAndRunIDAnchor(cursor, runPageNamespace, filters)
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
		beforeCreatedAt,
		beforeID,
		size+1,
	)
	if err != nil {
		return pagination.Page[run.Run]{}, err
	}
	if err := validateRunPage(rows, filter, beforeCreatedAt, beforeID, size+1); err != nil {
		return pagination.Page[run.Run]{}, err
	}
	rows = slices.Clone(rows)
	return pagination.PageOf(rows, size, runPageNamespace, filters, func(run run.Run) []string {
		return []string{strconv.FormatInt(run.CreatedAt().UnixNano(), 10), run.ID()}
	})
}

func (f RunPageFilter) validate() error {
	if f.SessionID != "" {
		if _, err := resourceid.ParseSession(f.SessionID); err != nil {
			return fmt.Errorf("sessions: query Runs page: %w", err)
		}
	}
	for _, status := range f.Statuses {
		if !status.Valid() {
			return fmt.Errorf("sessions: query Runs page has invalid status %q", status)
		}
	}
	return nil
}

func validateRunPage(rows []run.Run, filter RunPageFilter, beforeCreatedAt int64, beforeID string, maximum int) error {
	if len(rows) > maximum {
		return fmt.Errorf("sessions: Run store returned %d rows, maximum %d", len(rows), maximum)
	}
	if err := validateRunCatalog(rows, filter.SessionID); err != nil {
		return err
	}
	for index, value := range rows {
		if len(filter.Statuses) > 0 && !slices.Contains(filter.Statuses, value.State().Status()) {
			return fmt.Errorf("sessions: Run %q does not match the status filter", value.ID())
		}
		if !filter.IncludeDescendants && value.Lineage().IsChild() {
			return fmt.Errorf("sessions: root Run page contains child %q", value.ID())
		}
		if beforeID != "" && !runFollowsPosition(value, beforeCreatedAt, beforeID) {
			return fmt.Errorf("sessions: Run %q does not follow the page cursor", value.ID())
		}
		if index > 0 && !runFollowsPosition(value, rows[index-1].CreatedAt().UnixNano(), rows[index-1].ID()) {
			return fmt.Errorf("sessions: Run %q is out of order after %q", value.ID(), rows[index-1].ID())
		}
	}
	return nil
}

func runFollowsPosition(value run.Run, createdAt int64, id string) bool {
	position := value.CreatedAt().UnixNano()
	return position < createdAt || position == createdAt && value.ID() < id
}

// normalizeStatuses puts a status set in one canonical order and drops repeats, so
// two requests asking for the same set mint the same cursor. Sorting is by the
// domain's own declaration order — the enum IS the order, so there is nothing else
// to agree with.
func normalizeStatuses(statuses []run.Status) []run.Status {
	if len(statuses) == 0 {
		return nil
	}
	normalized := slices.Clone(statuses)
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

// statusFilter is the normalized status set as one cursor filter value. The empty
// set is every status, which is a different collection from any explicit set — and
// it reads as such, since no status is spelled "".
func statusFilter(statuses []run.Status) string {
	names := make([]string, 0, len(statuses))
	for _, status := range statuses {
		names = append(names, status.String())
	}
	return strings.Join(names, ",")
}

// ListPendingInterruptPage returns one page of the durable waiting sets, the
// longest wait first, continuing after cursor. An empty sessionID pages across
// every session and an empty rootRunID every waiting tree; given together they must
// both match.
//
// The page unit is a whole set, never an interrupt: resume validates and
// consumes a set in one transaction, so half a set is a resume nobody can attempt.
// Sets are one row each, which is what makes "never split" a property of the
// storage rather than a rule this read has to remember.
//
// caller is what the requester declared it can follow. A set whose Run
// publishes more than that is REFUSED — [run.ErrInsufficientCapabilities] — rather
// than returned with the parts the caller understands: answering a
// trimmed set would leave the rest of it open forever, and the run would stay
// waiting on interrupts the requester believes it resolved.
//
// rootRunID must name a root. A child id is [transcript.ErrNotRoot], because the
// set it belongs to exists — under the root — and an empty page would say otherwise.
func (c *QueryCoordinator) ListPendingInterruptPage(ctx context.Context, sessionID, rootRunID string, caller run.Capabilities, cursor string, limit pagination.RequestedLimit) (pagination.Page[runs.Pending], error) {
	caller = caller.Clone()
	if err := caller.Validate(); err != nil {
		return pagination.Page[runs.Pending]{}, fmt.Errorf("sessions: query interrupts page caller capabilities: %w", err)
	}
	if sessionID != "" {
		if _, err := resourceid.ParseSession(sessionID); err != nil {
			return pagination.Page[runs.Pending]{}, fmt.Errorf("sessions: query interrupts page: %w", err)
		}
	}
	if rootRunID != "" {
		if _, err := resourceid.ParseRun(rootRunID); err != nil {
			return pagination.Page[runs.Pending]{}, fmt.Errorf("sessions: query interrupts page: %w", err)
		}
	}
	filters := []string{sessionID, rootRunID}
	afterCreatedAt, afterID, err := timeAndRunIDAnchor(cursor, interruptPageNamespace, filters)
	if err != nil {
		return pagination.Page[runs.Pending]{}, err
	}
	size, err := limit.Resolve(interruptPageLimit)
	if err != nil {
		return pagination.Page[runs.Pending]{}, err
	}
	if requireRootErr := c.requireRoot(ctx, rootRunID); requireRootErr != nil {
		return pagination.Page[runs.Pending]{}, requireRootErr
	}
	rows, err := c.interrupts.ListPage(ctx, sessionID, rootRunID, afterCreatedAt, afterID, size+1)
	if err != nil {
		return pagination.Page[runs.Pending]{}, err
	}
	if err := validatePendingInterruptPage(rows, sessionID, rootRunID, afterCreatedAt, afterID, size+1); err != nil {
		return pagination.Page[runs.Pending]{}, err
	}
	owned := make([]runs.Pending, len(rows))
	for index, pending := range rows {
		owned[index] = pending.Clone()
	}
	rows = owned
	page, err := pagination.PageOf(rows, size, interruptPageNamespace, filters, func(pending runs.Pending) []string {
		return []string{strconv.FormatInt(pending.CreatedAt.UnixNano(), 10), pending.RootRunID}
	})
	if err != nil {
		return pagination.Page[runs.Pending]{}, err
	}
	for _, pending := range page.Rows {
		if gap := pending.Capabilities.MissingFrom(caller); !gap.IsEmpty() {
			return pagination.Page[runs.Pending]{}, &run.InsufficientCapabilitiesError{RunID: pending.RootRunID, Missing: gap}
		}
	}
	return page, nil
}

func validatePendingInterruptPage(rows []runs.Pending, sessionID, rootRunID string, afterCreatedAt int64, afterID string, maximum int) error {
	if len(rows) > maximum {
		return fmt.Errorf("sessions: interrupt store returned %d rows, maximum %d", len(rows), maximum)
	}
	if err := validatePendingCatalog(rows, sessionID); err != nil {
		return err
	}
	for index, pending := range rows {
		if rootRunID != "" && pending.RootRunID != rootRunID {
			return fmt.Errorf("sessions: pending set %q does not match root Run filter %q", pending.RootRunID, rootRunID)
		}
		if afterID != "" && !pendingFollowsPosition(pending, afterCreatedAt, afterID) {
			return fmt.Errorf("sessions: pending set %q does not follow the page cursor", pending.RootRunID)
		}
		if index > 0 && !pendingFollowsPosition(pending, rows[index-1].CreatedAt.UnixNano(), rows[index-1].RootRunID) {
			return fmt.Errorf("sessions: pending set %q is out of order after %q", pending.RootRunID, rows[index-1].RootRunID)
		}
	}
	return nil
}

func pendingFollowsPosition(pending runs.Pending, createdAt int64, rootRunID string) bool {
	position := pending.CreatedAt.UnixNano()
	return position > createdAt || position == createdAt && pending.RootRunID > rootRunID
}

// requireRoot refuses a run filter that names a child. An empty filter names no run
// and is not checked; a filter naming nothing that exists is left to the page, which
// returns none — "no such run" and "that run has nothing waiting" are the same
// answer to the caller, while "you named a child" is not.
func (c *QueryCoordinator) requireRoot(ctx context.Context, runID string) error {
	if runID == "" {
		return nil
	}
	if _, err := resourceid.ParseRun(runID); err != nil {
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

// timeAndRunIDAnchor reads a decoded cursor's (timestamp, Run identity) sort
// position. The Run identity is what makes the order total: two rows can share
// a nanosecond, and a timestamp-only bound would then drop one or return it
// twice. Both current consumers page Run-owned records, so accepting a generic
// string here would weaken the identity boundary immediately after decoding.
func timeAndRunIDAnchor(cursor, method string, filters []string) (int64, string, error) {
	anchor, err := pagination.Decode(cursor, method, filters)
	if err != nil {
		return 0, "", err
	}
	if len(anchor) == 0 {
		return 0, "", nil
	}
	if len(anchor) != 2 {
		return 0, "", pagination.ErrInvalidCursor
	}
	stamp, err := strconv.ParseInt(anchor[0], 10, 64)
	if err != nil {
		return 0, "", pagination.ErrInvalidCursor
	}
	if _, err := resourceid.ParseRun(anchor[1]); err != nil {
		return 0, "", pagination.ErrInvalidCursor
	}
	return stamp, anchor[1], nil
}
