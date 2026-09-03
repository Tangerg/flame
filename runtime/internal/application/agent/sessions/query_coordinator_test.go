package sessions

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type fakeTranscript struct {
	items []transcript.SequencedItem
	trees map[string][]string

	session       string
	run           string
	runTree       string
	order         transcript.SequenceOrder
	afterSequence int64
	limit         int
}

func (f *fakeTranscript) PageSessionItems(_ context.Context, sessionID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	f.session = sessionID
	return f.page(order, fromSequence, limit, func(transcript.Item) bool { return true })
}

func (f *fakeTranscript) PageRunItems(_ context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	f.run = runID
	return f.page(order, fromSequence, limit, func(item transcript.Item) bool { return item.RunID() == runID })
}

func (f *fakeTranscript) PageRunTreeItems(_ context.Context, runID string, order transcript.SequenceOrder, fromSequence int64, limit int) ([]transcript.SequencedItem, error) {
	f.runTree = runID
	runIDs := append([]string{runID}, f.trees[runID]...)
	return f.page(order, fromSequence, limit, func(item transcript.Item) bool {
		return slices.Contains(runIDs, item.RunID())
	})
}

// page seeks the way the store does: zero is no anchor in either direction, and
// newest-first is the same sequence read from the other end.
func (f *fakeTranscript) page(order transcript.SequenceOrder, fromSequence int64, limit int, keep func(transcript.Item) bool) ([]transcript.SequencedItem, error) {
	f.order, f.afterSequence, f.limit = order, fromSequence, limit
	rows := slices.Clone(f.items)
	if order == transcript.NewestFirst {
		slices.Reverse(rows)
	}
	var out []transcript.SequencedItem
	for _, entry := range rows {
		if !keep(entry.Item) {
			continue
		}
		if fromSequence > 0 {
			if order == transcript.NewestFirst && entry.Sequence >= fromSequence {
				continue
			}
			if order == transcript.OldestFirst && entry.Sequence <= fromSequence {
				continue
			}
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, entry)
	}
	return out, nil
}

// fakeSessions answers only whether a session exists. Every session an item test
// names exists unless it is listed as missing.
type fakeSessions struct{ missing []string }

func (f *fakeSessions) Exists(_ context.Context, sessionID string) (bool, error) {
	return !slices.Contains(f.missing, sessionID), nil
}

// fakeRuns is the Run record the item page threads its items against, and the
// durable history the run page seeks through. Both seek the way the store does:
// an empty id is no anchor — the first page — and anything else is strictly past
// the last row of the page before it, which for the history means EARLIER.
type fakeRuns struct {
	runs []run.Run
	// history is newest first, the order the store returns.
	history []run.Run

	session         string
	requested       []string
	statuses        []run.Status
	descendants     bool
	beforeCreatedAt int64
	beforeRunID     string
	limit           int
}

type rawRunPageReader struct {
	*fakeRuns
	page []run.Run
}

func (r *rawRunPageReader) PageRuns(context.Context, string, []run.Status, bool, int64, string, int) ([]run.Run, error) {
	return r.page, nil
}

func (f *fakeRuns) Run(_ context.Context, runID string) (run.Run, bool, error) {
	for _, run := range f.history {
		if run.ID() == runID {
			return run, true, nil
		}
	}
	return run.Run{}, false, nil
}

func (f *fakeRuns) PageRuns(_ context.Context, sessionID string, statuses []run.Status, includeDescendants bool, beforeCreatedAt int64, beforeRunID string, limit int) ([]run.Run, error) {
	f.session, f.statuses, f.descendants = sessionID, statuses, includeDescendants
	f.beforeCreatedAt, f.beforeRunID, f.limit = beforeCreatedAt, beforeRunID, limit
	var out []run.Run
	for _, run := range f.history {
		if sessionID != "" && run.SessionID() != sessionID {
			continue
		}
		if !includeDescendants && run.Lineage().IsChild() {
			continue
		}
		if !seeksBefore(run.CreatedAt().UnixNano(), run.ID(), beforeCreatedAt, beforeRunID) {
			continue
		}
		if len(statuses) > 0 && !slices.Contains(statuses, run.State().Status()) {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, run)
	}
	return out, nil
}

func (f *fakeRuns) RunsWithAncestors(_ context.Context, runIDs []string) ([]run.Run, error) {
	f.requested = runIDs
	wanted := slices.Clone(runIDs)
	for index := 0; index < len(wanted); index++ {
		for _, run := range f.runs {
			if run.ID() == wanted[index] && run.Lineage().ParentRunID != "" && !slices.Contains(wanted, run.Lineage().ParentRunID) {
				wanted = append(wanted, run.Lineage().ParentRunID)
			}
		}
	}
	var out []run.Run
	for _, run := range f.runs {
		if slices.Contains(wanted, run.ID()) {
			out = append(out, run)
		}
	}
	return out, nil
}

type fakeInterrupts struct {
	pending []runs.Pending

	session        string
	rootRun        string
	afterCreatedAt int64
	afterRunID     string
	limit          int
}

func newQueryCoordinator(t *testing.T, deps QueryDependencies) *QueryCoordinator {
	t.Helper()
	if deps.Transcript == nil {
		deps.Transcript = &fakeTranscript{}
	}
	if deps.Interrupts == nil {
		deps.Interrupts = &fakeInterrupts{}
	}
	if deps.Runs == nil {
		deps.Runs = &fakeRuns{}
	}
	if deps.Sessions == nil {
		deps.Sessions = &fakeSessions{}
	}
	coordinator, err := NewQueryCoordinator(deps)
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}

func TestNewQueryCoordinatorRejectsIncompleteDependencies(t *testing.T) {
	complete := QueryDependencies{
		Transcript: &fakeTranscript{}, Interrupts: &fakeInterrupts{},
		Runs: &fakeRuns{}, Sessions: &fakeSessions{},
	}
	if _, err := NewQueryCoordinator(QueryDependencies{
		Interrupts: complete.Interrupts, Runs: complete.Runs, Sessions: complete.Sessions,
	}); err == nil || !strings.Contains(err.Error(), "transcript reader is required") {
		t.Fatalf("missing transcript error = %v", err)
	}
	var typedNilTranscript *fakeTranscript
	complete.Transcript = typedNilTranscript
	if _, err := NewQueryCoordinator(complete); err == nil || !strings.Contains(err.Error(), "transcript reader is required") {
		t.Fatalf("typed-nil transcript error = %v", err)
	}
	complete.Transcript = &fakeTranscript{}
	var typedNilPlan *PlanCoordinator
	complete.Plan = typedNilPlan
	if _, err := NewQueryCoordinator(complete); err == nil || !strings.Contains(err.Error(), "Plan reader must not be typed nil") {
		t.Fatalf("typed-nil Plan error = %v", err)
	}
}

func (f *fakeInterrupts) ListPage(_ context.Context, sessionID, rootRunID string, afterCreatedAt int64, afterRunID string, limit int) ([]runs.Pending, error) {
	f.session, f.rootRun = sessionID, rootRunID
	f.afterCreatedAt, f.afterRunID, f.limit = afterCreatedAt, afterRunID, limit
	var out []runs.Pending
	for _, pending := range f.pending {
		if !seeksPast(pending.CreatedAt.UnixNano(), pending.RootRunID, afterCreatedAt, afterRunID) {
			continue
		}
		if rootRunID != "" && pending.RootRunID != rootRunID {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, pending)
	}
	return out, nil
}

// seeksPast is the store's own seek predicate: order by (timestamp, id), and treat
// a zero pair as the first page rather than as a position before every row.
func seeksPast(at int64, id string, afterAt int64, afterID string) bool {
	if afterAt == 0 && afterID == "" {
		return true
	}
	return at > afterAt || (at == afterAt && id > afterID)
}

// seeksBefore is the same rule for a newest-first read, where continuing means
// going back in time. An empty id is the first page: unlike a timestamp, it cannot
// be confused with a position.
func seeksBefore(at int64, id string, beforeAt int64, beforeID string) bool {
	if beforeID == "" {
		return true
	}
	return at < beforeAt || (at == beforeAt && id < beforeID)
}

// sequencedItems builds a session's items, every one belonging to run_1, so a page
// of them has a run to be threaded onto.
func sequencedItems(count int) []transcript.SequencedItem {
	out := make([]transcript.SequencedItem, 0, count)
	for i := 1; i <= count; i++ {
		out = append(out, transcript.SequencedItem{
			Sequence: int64(i),
			Item:     testsupport.MustRestoreItem(testsupport.ItemInput{ID: "it_" + strconv.Itoa(i), RunID: "run_1"}),
		})
	}
	return out
}

func queryRunIDs(runs []run.Run) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		out = append(out, run.ID())
	}
	return out
}

func queryRun(id string) run.Run {
	return testsupport.MustRestoreRun(run.Snapshot{ID: id})
}

func TestCoordinatorReadsDelegateToProjections(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(1)}
	runStore := &fakeRuns{runs: []run.Run{queryRun("run_1")}}
	ints := &fakeInterrupts{pending: []runs.Pending{{RootRunID: "run_1"}}}
	c := newQueryCoordinator(t, QueryDependencies{Transcript: tx, Interrupts: ints, Runs: runStore, Sessions: &fakeSessions{}})

	page, err := c.ListItemPage(ctx, Items("ses_1"), transcript.OldestFirst, "", pagination.DefaultLimit())
	if err != nil || len(page.Items) != 1 || len(page.Runs) != 1 || tx.session != "ses_1" {
		t.Fatalf("ListItemPage items=%d runs=%d session=%q err=%v", len(page.Items), len(page.Runs), tx.session, err)
	}
	if !slices.Equal(runStore.requested, []string{"run_1"}) {
		t.Fatalf("threaded runs = %v, want only the run the page's items belong to", runStore.requested)
	}

	pending, err := c.ListPendingInterruptPage(ctx, "ses_2", "", run.Capabilities{}, "", pagination.DefaultLimit())
	if err != nil || len(pending.Rows) != 1 || ints.session != "ses_2" {
		t.Fatalf("ListPendingInterruptPage pending=%d session=%q err=%v", len(pending.Rows), ints.session, err)
	}
}

// The page is cut by the query, not after the fact: the read asks for exactly one
// row more than it will return, which is both how "there is more" is known and
// what keeps a long session's history out of memory.
// It is also the items cursor namespace's fixed-order and next-page-direction fixture.
func TestListItemPageBoundsTheQueryAndSeeksPastTheAnchor(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(5)}
	c := newQueryCoordinator(t, QueryDependencies{Transcript: tx, Runs: &fakeRuns{}, Sessions: &fakeSessions{}})

	first, err := c.ListItemPage(ctx, Items("ses_1"), transcript.OldestFirst, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if tx.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", tx.limit)
	}
	if len(first.Items) != 2 || first.Items[0].ID() != "it_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two items and a cursor", first)
	}

	second, err := c.ListItemPage(ctx, Items("ses_1"), transcript.OldestFirst, first.NextCursor, explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if tx.afterSequence != 2 {
		t.Fatalf("second page sought past %d, want the first page's last position", tx.afterSequence)
	}
	if len(second.Items) != 2 || second.Items[0].ID() != "it_3" {
		t.Fatalf("second page = %+v, want it_3 onward", second)
	}

	last, err := c.ListItemPage(ctx, Items("ses_1"), transcript.OldestFirst, second.NextCursor, explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("last page: %v", err)
	}
	if len(last.Items) != 1 || last.NextCursor != "" {
		t.Fatalf("last page = %+v, want the tail and no cursor", last)
	}
}

// A cursor from another session would page this one against positions it never
// enumerated. Restarting from the top instead of refusing would hand the client
// rows it had already read, as if they were new. It is the items cursor-binding
// fixture.
func TestListItemPageRefusesAForeignCursor(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(5)}
	c := newQueryCoordinator(t, QueryDependencies{
		Transcript: tx,
		Runs:       &fakeRuns{history: []run.Run{queryRun("run_1")}},
		Sessions:   &fakeSessions{},
	})

	other, err := c.ListItemPage(ctx, Items("ses_other"), transcript.OldestFirst, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("other session page: %v", err)
	}
	if _, listItemPageErr := c.ListItemPage(ctx, Items("ses_1"), transcript.OldestFirst, other.NextCursor, explicitPageLimit(t, 2)); !errors.Is(listItemPageErr, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-session cursor err = %v, want ErrInvalidCursor", listItemPageErr)
	}
	if _, listItemPageErr := c.ListItemPage(ctx, Items("ses_1"), transcript.OldestFirst, "not-a-cursor", explicitPageLimit(t, 2)); !errors.Is(listItemPageErr, pagination.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", listItemPageErr)
	}

	// Direction is part of the query, not a display preference applied afterwards: an
	// anchor from a forward page names a position a backward page never reaches.
	forward, err := c.ListItemPage(ctx, Items("ses_1"), transcript.OldestFirst, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("forward page: %v", err)
	}
	if _, listItemPageErr := c.ListItemPage(ctx, Items("ses_1"), transcript.NewestFirst, forward.NextCursor, explicitPageLimit(t, 2)); !errors.Is(listItemPageErr, pagination.ErrInvalidCursor) {
		t.Fatalf("reversed-direction cursor err = %v, want ErrInvalidCursor", listItemPageErr)
	}

	// A run scope is a different collection from the session that contains it, even
	// when every item in the session belongs to that run.
	runScoped, err := c.ListItemPage(ctx, RunItems("run_1"), transcript.OldestFirst, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("run page: %v", err)
	}
	if _, err := c.ListItemPage(ctx, Items("ses_1"), transcript.OldestFirst, runScoped.NextCursor, explicitPageLimit(t, 2)); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("run cursor on the session page err = %v, want ErrInvalidCursor", err)
	}
}

// TestListItemPageWalksBackwardFromTheTail is the items read's other direction: the same
// durable sequence read from the end. A long session's first screen is its tail, and
// paging forward to reach it would read everything before it first.
func TestListItemPageWalksBackwardFromTheTail(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{items: sequencedItems(5)}
	c := newQueryCoordinator(t, QueryDependencies{Transcript: tx, Runs: &fakeRuns{}, Sessions: &fakeSessions{}})

	first, err := c.ListItemPage(ctx, Items("ses_1"), transcript.NewestFirst, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Items) != 2 || first.Items[0].ID() != "it_5" || first.Items[1].ID() != "it_4" {
		t.Fatalf("first page = %+v, want the last two items newest first", first.Items)
	}

	second, err := c.ListItemPage(ctx, Items("ses_1"), transcript.NewestFirst, first.NextCursor, explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if tx.afterSequence != 4 {
		t.Fatalf("second page sought from %d, want the first page's last position", tx.afterSequence)
	}
	if len(second.Items) != 2 || second.Items[0].ID() != "it_3" {
		t.Fatalf("second page = %+v, want it_3 backwards", second.Items)
	}
}

// TestListItemPageScopedToARunReadsOnlyThatRun pins the run scope: the items of one
// run, resolved from the run id alone. A caller holding a runId does not have to
// discover the session to read what that run did.
func TestListItemPageScopedToARunReadsOnlyThatRun(t *testing.T) {
	ctx := context.Background()
	items := sequencedItems(3)
	snapshot := items[2].Item.Snapshot()
	snapshot.Identity.RunID = "run_2"
	items[2].Item = testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: snapshot.Identity.SessionID, RunID: snapshot.Identity.RunID,
		ID: snapshot.Identity.ItemID, OccurredAt: snapshot.Identity.OccurredAt,
		Status: snapshot.Status, Kind: snapshot.Kind, Content: snapshot.Content,
	})
	tx := &fakeTranscript{items: items}
	runs := &fakeRuns{
		runs:    []run.Run{queryRun("run_1"), queryRun("run_2")},
		history: []run.Run{queryRun("run_1"), queryRun("run_2")},
	}
	c := newQueryCoordinator(t, QueryDependencies{Transcript: tx, Runs: runs, Sessions: &fakeSessions{}})

	page, err := c.ListItemPage(ctx, RunItems("run_2"), transcript.OldestFirst, "", pagination.DefaultLimit())
	if err != nil {
		t.Fatalf("run page: %v", err)
	}
	if tx.run != "run_2" || tx.session != "" {
		t.Fatalf("read run=%q session=%q, want only the run scope", tx.run, tx.session)
	}
	if len(page.Items) != 1 || page.Items[0].ID() != "it_3" {
		t.Fatalf("page = %+v, want only run_2's item", page.Items)
	}
	// The page carries the runs its own items reference — not the session's list.
	if !slices.Equal(runs.requested, []string{"run_2"}) {
		t.Fatalf("threaded runs = %v, want only run_2", runs.requested)
	}
}

func TestListItemPageScopesASubtreeAndIncludesAncestors(t *testing.T) {
	root := testsupport.MustRestoreRun(run.Snapshot{ID: "run_root"})
	child := testsupport.MustRestoreRun(run.Snapshot{ID: "run_child", Lineage: run.Lineage{SpawnedByItemID: "item_spawn_child",
		ParentRunID: root.ID(), RootRunID: root.ID()}})

	grandchild := testsupport.MustRestoreRun(run.Snapshot{ID: "run_grandchild", Lineage: run.Lineage{SpawnedByItemID: "item_spawn_grandchild",
		ParentRunID: child.ID(), RootRunID: root.ID()}})

	sibling := testsupport.MustRestoreRun(run.Snapshot{ID: "run_sibling", Lineage: run.Lineage{SpawnedByItemID: "item_spawn_sibling",
		ParentRunID: root.ID(), RootRunID: root.ID()}})

	tx := &fakeTranscript{
		items: []transcript.SequencedItem{
			{Sequence: 1, Item: testsupport.MustRestoreItem(testsupport.ItemInput{ID: "item_root", RunID: root.ID()})},
			{Sequence: 2, Item: testsupport.MustRestoreItem(testsupport.ItemInput{ID: "item_child", RunID: child.ID()})},
			{Sequence: 3, Item: testsupport.MustRestoreItem(testsupport.ItemInput{ID: "item_grandchild", RunID: grandchild.ID()})},
			{Sequence: 4, Item: testsupport.MustRestoreItem(testsupport.ItemInput{ID: "item_sibling", RunID: sibling.ID()})},
		},
		trees: map[string][]string{child.ID(): {grandchild.ID()}},
	}
	runs := &fakeRuns{
		runs:    []run.Run{grandchild, child, root, sibling},
		history: []run.Run{grandchild, sibling, child, root},
	}
	c := newQueryCoordinator(t, QueryDependencies{Transcript: tx, Runs: runs, Sessions: &fakeSessions{}})

	page, err := c.ListItemPage(t.Context(), RunTreeItems(child.ID()), transcript.OldestFirst, "", pagination.DefaultLimit())
	if err != nil {
		t.Fatalf("subtree page: %v", err)
	}
	if tx.runTree != child.ID() || tx.run != "" {
		t.Fatalf("read subtree=%q exact=%q, want only child subtree", tx.runTree, tx.run)
	}
	if len(page.Items) != 2 {
		t.Fatalf("subtree items = %+v, want child and grandchild only", page.Items)
	}
	if got := []string{page.Items[0].ID(), page.Items[1].ID()}; !slices.Equal(got, []string{"item_child", "item_grandchild"}) {
		t.Fatalf("subtree items = %v, want child and grandchild only", got)
	}
	if got := queryRunIDs(page.Runs); !slices.Equal(got, []string{grandchild.ID(), child.ID(), root.ID()}) {
		t.Fatalf("page runs = %v, want direct runs plus ancestor closure", got)
	}
	if !slices.Equal(runs.requested, []string{child.ID(), grandchild.ID()}) {
		t.Fatalf("directly referenced runs = %v, want page item sources only", runs.requested)
	}

	first, err := c.ListItemPage(t.Context(), RunTreeItems(child.ID()), transcript.OldestFirst, "", explicitPageLimit(t, 1))
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first subtree page = (%+v, %v), want a cursor", first, err)
	}
	if _, err := c.ListItemPage(t.Context(), RunItems(child.ID()), transcript.OldestFirst, first.NextCursor, explicitPageLimit(t, 1)); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("subtree cursor on exact-run page = %v, want ErrInvalidCursor", err)
	}
}

// TestListItemPageRefusesAScopeThatNamesNothing keeps an empty page from standing in
// for a wrong id. "This session has no items" and "there is no such session" are
// different facts, and a client that cannot tell them apart will show an empty
// timeline for a typo.
func TestListItemPageRefusesAScopeThatNamesNothing(t *testing.T) {
	ctx := context.Background()
	c := newQueryCoordinator(t, QueryDependencies{
		Transcript: &fakeTranscript{items: sequencedItems(3)},
		Runs:       &fakeRuns{history: []run.Run{queryRun("run_1")}},
		Sessions:   &fakeSessions{missing: []string{"ses_gone"}},
	})

	if _, err := c.ListItemPage(ctx, Items("ses_gone"), transcript.OldestFirst, "", pagination.DefaultLimit()); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("missing session err = %v, want session.ErrNotFound", err)
	}
	if _, err := c.ListItemPage(ctx, RunItems("run_gone"), transcript.OldestFirst, "", pagination.DefaultLimit()); !errors.Is(err, transcript.ErrRunNotFound) {
		t.Fatalf("missing run err = %v, want transcript.ErrRunNotFound", err)
	}
}

func TestListItemPageRejectsAnUnknownOrder(t *testing.T) {
	tx := &fakeTranscript{items: sequencedItems(1)}
	c := newQueryCoordinator(t, QueryDependencies{Transcript: tx, Runs: &fakeRuns{}, Sessions: &fakeSessions{}})

	if _, err := c.ListItemPage(t.Context(), Items("ses_1"), transcript.SequenceOrder("ascending"), "", explicitPageLimit(t, 1)); err == nil {
		t.Fatal("unknown order returned no error")
	}
	if tx.order != "" {
		t.Fatalf("unknown order reached transcript store as %q", tx.order)
	}
}

func TestSequenceAnchorRequiresAPositiveSequence(t *testing.T) {
	for _, anchor := range [][]string{{"0"}, {"-1"}, {"not-a-sequence"}, {"1", "extra"}} {
		if _, err := sequenceAnchor(anchor); !errors.Is(err, pagination.ErrInvalidCursor) {
			t.Fatalf("sequenceAnchor(%q) err = %v, want ErrInvalidCursor", anchor, err)
		}
	}
	if got, err := sequenceAnchor([]string{"1"}); err != nil || got != 1 {
		t.Fatalf("sequenceAnchor(1) = (%d, %v)", got, err)
	}
}

func TestTimeAndRunIDAnchorRejectsForgedIdentityMaterial(t *testing.T) {
	const cursorTimestampNanos = int64(42)
	forged, err := pagination.Encode(
		runPageNamespace,
		nil,
		[]string{strconv.FormatInt(cursorTimestampNanos, 10), "run forged"},
	)
	if err != nil {
		t.Fatalf("encode structurally valid forged cursor: %v", err)
	}
	if _, _, err := timeAndRunIDAnchor(forged, runPageNamespace, nil); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("forged Run identity err = %v, want ErrInvalidCursor", err)
	}
}

// testSessionRunHistory builds the run page's rows in the order the store
// returns them: newest admission first, one nanosecond apart. States cycle
// through the three lifecycle positions so a status filter has something to
// exclude.
func testSessionRunHistory(ids ...string) []run.Run {
	return testRunHistory("ses_1", ids...)
}

func testRunHistory(sessionID string, ids ...string) []run.Run {
	states := [...]run.State{run.Running, run.Waiting, run.Completed}
	out := make([]run.Run, 0, len(ids))
	for i, id := range ids {
		state := states[i%len(states)]
		var outcome *run.Outcome
		if state.IsTerminal() {
			value := run.OutcomeCompleted
			outcome = &value
		}
		record := testsupport.MustRestoreRun(run.Snapshot{ID: id, SessionID: sessionID, State: state,
			Outcome: outcome, CreatedAt: time.Unix(0, int64(len(ids)-i)).UTC()})
		out = append(out, record)
	}
	return out
}

func testSessionPendingRuns(ids ...string) []runs.Pending {
	out := make([]runs.Pending, 0, len(ids))
	for i, id := range ids {
		out = append(out, runs.Pending{
			RootRunID: id, SessionID: "ses_1", CreatedAt: time.Unix(0, int64(i+1)).UTC(),
		})
	}
	return out
}

// TestListPendingInterruptPageRefusesACallerThatCannotFollowTheRun is the deferred
// half of the capabilities rule: a waiting set belongs to a run with a frozen contract,
// and a caller that cannot follow that contract is refused the set — never handed
// the parts it happens to understand.
//
// A trimmed set is worse than an error: the client would answer what it received,
// resume would consume it as the whole set, and the run would sit waiting on
// interrupts the client believes it resolved.
func TestListPendingInterruptPageRefusesACallerThatCannotFollowTheRun(t *testing.T) {
	ctx := context.Background()
	waiting := testSessionPendingRuns("run_1")
	waiting[0].Capabilities = run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
	}
	c := newQueryCoordinator(t, QueryDependencies{
		Transcript: &fakeTranscript{},
		Runs:       &fakeRuns{history: []run.Run{queryRun("run_1")}},
		Interrupts: &fakeInterrupts{pending: waiting},
		Sessions:   &fakeSessions{},
	})

	answersOnlyApprovals := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	}
	if _, err := c.ListPendingInterruptPage(ctx, "ses_1", "", answersOnlyApprovals, "", pagination.DefaultLimit()); !errors.Is(err, run.ErrInsufficientCapabilities) {
		t.Fatalf("partial caller err = %v, want ErrInsufficientCapabilities", err)
	}

	full := run.Capabilities{
		InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
	}
	page, err := c.ListPendingInterruptPage(ctx, "ses_1", "", full, "", pagination.DefaultLimit())
	if err != nil || len(page.Rows) != 1 {
		t.Fatalf("covering caller = %d rows, %v; want the whole set", len(page.Rows), err)
	}
}

// TestListPendingInterruptPageFiltersByRootAndRefusesAChild pins the run filter: it
// narrows to one waiting tree, and a child id is a refusal rather than an empty page
// — the set the caller wants exists, under the root, so "nothing here" would send it
// looking in the wrong place.
func TestListPendingInterruptPageFiltersByRootAndRefusesAChild(t *testing.T) {
	ctx := context.Background()
	ints := &fakeInterrupts{pending: testSessionPendingRuns("run_1", "run_2")}
	c := newQueryCoordinator(t, QueryDependencies{
		Transcript: &fakeTranscript{},
		Runs: &fakeRuns{history: []run.Run{
			queryRun("run_1"),
			testsupport.MustRestoreRun(run.Snapshot{ID: "run_child", Lineage: run.Lineage{
				SpawnedByItemID: "it_spawn", ParentRunID: "run_1", RootRunID: "run_1",
			}}),
		}},
		Interrupts: ints,
		Sessions:   &fakeSessions{},
	})

	page, err := c.ListPendingInterruptPage(ctx, "", "run_1", run.Capabilities{}, "", pagination.DefaultLimit())
	if err != nil {
		t.Fatalf("root-filtered page: %v", err)
	}
	if ints.rootRun != "run_1" || len(page.Rows) != 1 || page.Rows[0].RootRunID != "run_1" {
		t.Fatalf("filtered page = %+v (asked %q), want only run_1's set", page.Rows, ints.rootRun)
	}

	if _, listPendingInterruptPageErr := c.ListPendingInterruptPage(ctx, "", "run_child", run.Capabilities{}, "", pagination.DefaultLimit()); !errors.Is(listPendingInterruptPageErr, transcript.ErrNotRoot) {
		t.Fatalf("child filter err = %v, want transcript.ErrNotRoot", listPendingInterruptPageErr)
	}

	// The filter is part of the cursor's identity: the same anchor against a
	// different filter names a position in a collection it never enumerated.
	unfiltered, err := c.ListPendingInterruptPage(ctx, "", "", run.Capabilities{}, "", explicitPageLimit(t, 1))
	if err != nil {
		t.Fatalf("unfiltered page: %v", err)
	}
	if _, err := c.ListPendingInterruptPage(ctx, "", "run_1", run.Capabilities{}, unfiltered.NextCursor, explicitPageLimit(t, 1)); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-filter cursor err = %v, want ErrInvalidCursor", err)
	}
}

// TestListRunPageWalksBackwardThroughHistory covers the runs query properties:
// the order is fixed (admission descending, tie-broken by id), the next page seeks
// strictly EARLIER than the last row rather than re-reading it, and "there is more"
// is only claimed when the over-fetch found it.
//
// Newest first is the direction the contract fixes, and it is the one a client
// needs: the run it is looking for is almost always the last one, and paging
// forward from the beginning of a long session would reach it last.
func TestListRunPageWalksBackwardThroughHistory(t *testing.T) {
	ctx := context.Background()
	runs := &fakeRuns{history: testSessionRunHistory("run_3", "run_2", "run_1")}
	c := newQueryCoordinator(t, QueryDependencies{Transcript: &fakeTranscript{}, Runs: runs, Sessions: &fakeSessions{}})

	first, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if runs.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", runs.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].ID() != "run_3" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want the two newest runs and a cursor", first.Rows)
	}

	second, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, first.NextCursor, explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if runs.beforeRunID != "run_2" {
		t.Fatalf("second page sought before %q, want the first page's last row", runs.beforeRunID)
	}
	if len(second.Rows) != 1 || second.Rows[0].ID() != "run_1" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}
}

func TestListRunPageRejectsBrokenStoreOutput(t *testing.T) {
	ordered := testSessionRunHistory("run_3", "run_2", "run_1")
	createdAt := time.Unix(10, 0).UTC()
	tieAscending := []run.Run{
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_a", SessionID: "ses_1", CreatedAt: createdAt}),
		testsupport.MustRestoreRun(run.Snapshot{ID: "run_b", SessionID: "ses_1", CreatedAt: createdAt}),
	}
	for name, page := range map[string][]run.Run{
		"invalid aggregate":     {{}},
		"duplicate identity":    {ordered[0], ordered[0]},
		"creation out of order": {ordered[1], ordered[0]},
		"id tie out of order":   tieAscending,
		"excess overfetch":      ordered,
	} {
		t.Run(name, func(t *testing.T) {
			reader := &rawRunPageReader{fakeRuns: &fakeRuns{}, page: page}
			coordinator := newQueryCoordinator(t, QueryDependencies{
				Transcript: &fakeTranscript{}, Runs: reader, Sessions: &fakeSessions{},
			})
			if _, err := coordinator.ListRunPage(t.Context(), RunPageFilter{IncludeDescendants: true}, "", explicitPageLimit(t, 1)); err == nil {
				t.Fatal("ListRunPage accepted broken store output")
			}
		})
	}
}

func TestListRunPageRejectsRowsOutsideFilterOrCursor(t *testing.T) {
	root := testsupport.MustRestoreRun(run.Snapshot{ID: "run_root", SessionID: "ses_other", CreatedAt: time.Unix(3, 0).UTC()})
	waiting := testsupport.MustRestoreRun(run.Snapshot{ID: "run_waiting", SessionID: "ses_1", State: run.Waiting, CreatedAt: time.Unix(2, 0).UTC()})
	child := testsupport.MustRestoreRun(run.Snapshot{
		ID: "run_child", SessionID: "ses_1", CreatedAt: time.Unix(1, 0).UTC(),
		Lineage: run.Lineage{SpawnedByItemID: "item_spawn", ParentRunID: "run_parent", RootRunID: "run_parent"},
	})
	for _, test := range []struct {
		name   string
		filter RunPageFilter
		value  run.Run
	}{
		{name: "Session", filter: RunPageFilter{SessionID: "ses_1", IncludeDescendants: true}, value: root},
		{name: "status", filter: RunPageFilter{Statuses: []run.Status{run.StatusRunning}, IncludeDescendants: true}, value: waiting},
		{name: "descendant", filter: RunPageFilter{}, value: child},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &rawRunPageReader{fakeRuns: &fakeRuns{}, page: []run.Run{test.value}}
			coordinator := newQueryCoordinator(t, QueryDependencies{
				Transcript: &fakeTranscript{}, Runs: reader, Sessions: &fakeSessions{},
			})
			if _, err := coordinator.ListRunPage(t.Context(), test.filter, "", explicitPageLimit(t, 1)); err == nil {
				t.Fatal("ListRunPage accepted a Run outside its filter")
			}
		})
	}

	reader := &rawRunPageReader{fakeRuns: &fakeRuns{}, page: []run.Run{root}}
	coordinator := newQueryCoordinator(t, QueryDependencies{
		Transcript: &fakeTranscript{}, Runs: reader, Sessions: &fakeSessions{},
	})
	cursor, err := pagination.Encode(runPageNamespace, []string{"", "", "true"}, []string{
		strconv.FormatInt(root.CreatedAt().UnixNano(), 10), root.ID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.ListRunPage(t.Context(), RunPageFilter{IncludeDescendants: true}, cursor, explicitPageLimit(t, 1)); err == nil {
		t.Fatal("ListRunPage accepted the cursor anchor again")
	}
	if _, err := coordinator.ListRunPage(t.Context(), RunPageFilter{Statuses: []run.Status{"other"}}, "", explicitPageLimit(t, 1)); err == nil {
		t.Fatal("ListRunPage accepted an invalid status filter")
	}
}

func TestListRunPageDoesNotExposeStoreSlice(t *testing.T) {
	reader := &rawRunPageReader{fakeRuns: &fakeRuns{}, page: testSessionRunHistory("run_2", "run_1")}
	coordinator := newQueryCoordinator(t, QueryDependencies{
		Transcript: &fakeTranscript{}, Runs: reader, Sessions: &fakeSessions{},
	})
	page, err := coordinator.ListRunPage(t.Context(), RunPageFilter{IncludeDescendants: true}, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatal(err)
	}
	page.Rows[0] = run.Run{}
	if got := reader.page[0].ID(); got != "run_2" {
		t.Fatalf("store row changed through page result: %q", got)
	}
}

// TestListRunPageReturnsEveryStatusUntilFiltered pins the default: the read is the
// whole history, not the work in progress. A page that hid finished runs would make
// "what did this session cost" unanswerable from the run record, which is the one
// place that knows.
func TestListRunPageReturnsEveryStatusUntilFiltered(t *testing.T) {
	ctx := context.Background()
	runs := &fakeRuns{history: testSessionRunHistory("run_3", "run_2", "run_1")}
	c := newQueryCoordinator(t, QueryDependencies{Transcript: &fakeTranscript{}, Runs: runs, Sessions: &fakeSessions{}})

	all, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, "", pagination.DefaultLimit())
	if err != nil || len(all.Rows) != 3 {
		t.Fatalf("unfiltered page = %d rows, %v; want every status", len(all.Rows), err)
	}

	// A filter is normalized before it selects rows OR mints a cursor: the same set
	// asked for in a different order is the same query, and it must page as one.
	filtered, err := c.ListRunPage(ctx, RunPageFilter{
		SessionID: "ses_1",
		Statuses: []run.Status{
			run.StatusWaiting, run.StatusRunning, run.StatusWaiting,
		},
	}, "", pagination.DefaultLimit())

	if err != nil {
		t.Fatalf("filtered page: %v", err)
	}
	if want := []run.Status{run.StatusRunning, run.StatusWaiting}; !slices.Equal(runs.statuses, want) {
		t.Fatalf("store filtered on %v, want the normalized %v", runs.statuses, want)
	}
	if len(filtered.Rows) != 2 {
		t.Fatalf("filtered page = %d rows, want the running and waiting ones", len(filtered.Rows))
	}
}

func TestListRunPageIncludesDescendantsAndBindsTheCursor(t *testing.T) {
	root := testsupport.MustRestoreRun(run.Snapshot{ID: "run_root", CreatedAt: time.Unix(0, 1).UTC()})
	child := testsupport.MustRestoreRun(run.Snapshot{ID: "run_child",
		CreatedAt: time.Unix(0, 2).UTC(), Lineage: run.Lineage{SpawnedByItemID: "item_spawn_child",
			ParentRunID: root.ID(), RootRunID: root.ID()}})

	grandchild := testsupport.MustRestoreRun(run.Snapshot{ID: "run_grandchild",
		CreatedAt: time.Unix(0, 3).UTC(), Lineage: run.Lineage{SpawnedByItemID: "item_spawn_grandchild",
			ParentRunID: child.ID(), RootRunID: root.ID()}})

	runs := &fakeRuns{history: []run.Run{grandchild, child, root}}
	c := newQueryCoordinator(t, QueryDependencies{Transcript: &fakeTranscript{}, Runs: runs, Sessions: &fakeSessions{}})

	roots, err := c.ListRunPage(t.Context(), RunPageFilter{}, "", pagination.DefaultLimit())
	if err != nil || !slices.Equal(queryRunIDs(roots.Rows), []string{root.ID()}) {
		t.Fatalf("root page = (%v, %v), want only root", queryRunIDs(roots.Rows), err)
	}
	all, err := c.ListRunPage(t.Context(), RunPageFilter{IncludeDescendants: true}, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("descendant page: %v", err)
	}
	if !runs.descendants || !slices.Equal(queryRunIDs(all.Rows), []string{grandchild.ID(), child.ID()}) || all.NextCursor == "" {
		t.Fatalf("descendant page = %+v, include=%t; want newest two and cursor", queryRunIDs(all.Rows), runs.descendants)
	}
	if _, err := c.ListRunPage(t.Context(), RunPageFilter{}, all.NextCursor, explicitPageLimit(t, 2)); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("descendant cursor on root page = %v, want ErrInvalidCursor", err)
	}
}

// TestListRunPageRefusesACursorFromAnotherQuery is the runs read's half of the cursor
// binding: an anchor is only meaningful against the ordering AND the filter that
// produced it. Continuing from a foreign one silently pages against positions this
// query never enumerated — the client is handed rows it already has, or none at
// all, with nothing to say why.
func TestListRunPageRefusesACursorFromAnotherQuery(t *testing.T) {
	ctx := context.Background()
	runHistory := append(
		testRunHistory("ses_other", "run_6", "run_5", "run_4"),
		testSessionRunHistory("run_3", "run_2", "run_1")...,
	)
	c := newQueryCoordinator(t, QueryDependencies{
		Transcript: &fakeTranscript{items: sequencedItems(5)},
		Runs:       &fakeRuns{history: runHistory},
		Interrupts: &fakeInterrupts{pending: testSessionPendingRuns("run_1", "run_2", "run_3")},
		Sessions:   &fakeSessions{},
	})

	otherSession, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_other"}, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("other session page: %v", err)
	}
	if _, listRunPageErr := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, otherSession.NextCursor, explicitPageLimit(t, 2)); !errors.Is(listRunPageErr, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-session cursor err = %v, want ErrInvalidCursor", listRunPageErr)
	}

	// Changing the status filter changes which rows exist, so the anchor no longer
	// names a position in the collection being paged.
	unfiltered, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("unfiltered page: %v", err)
	}
	if _, listRunPageErr := c.ListRunPage(ctx, RunPageFilter{
		SessionID: "ses_1",
		Statuses:  []run.Status{run.StatusRunning},
	}, unfiltered.NextCursor, explicitPageLimit(t, 2)); !errors.Is(listRunPageErr, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-filter cursor err = %v, want ErrInvalidCursor", listRunPageErr)
	}

	// The interrupt page is scoped the same way and ordered by a timestamp too, so
	// only the query namespace tells the two apart.
	interruptPage, err := c.ListPendingInterruptPage(ctx, "ses_1", "", run.Capabilities{}, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("interrupt page: %v", err)
	}
	if _, listRunPageErr := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, interruptPage.NextCursor, explicitPageLimit(t, 2)); !errors.Is(listRunPageErr, pagination.ErrInvalidCursor) {
		t.Fatalf("cross-query cursor err = %v, want ErrInvalidCursor", listRunPageErr)
	}

	itemPage, err := c.ListItemPage(ctx, Items("ses_1"), transcript.OldestFirst, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("item page: %v", err)
	}
	if _, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, itemPage.NextCursor, explicitPageLimit(t, 2)); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("item cursor on the run page err = %v, want ErrInvalidCursor", err)
	}
}

// TestListPendingInterruptPagePagesOldestFirst is the same three properties for
// the interrupts read, whose order the contract fixes as oldest first: a
// resumable run that keeps sinking below the page boundary is one nobody answers.
func TestListPendingInterruptPagePagesOldestFirst(t *testing.T) {
	ctx := context.Background()
	ints := &fakeInterrupts{pending: testSessionPendingRuns("run_1", "run_2", "run_3")}
	c := newQueryCoordinator(t, QueryDependencies{
		Transcript: &fakeTranscript{},
		Runs:       &fakeRuns{history: testSessionRunHistory("run_3", "run_2", "run_1")},
		Interrupts: ints,
		Sessions:   &fakeSessions{},
	})

	first, err := c.ListPendingInterruptPage(ctx, "ses_1", "", run.Capabilities{}, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if ints.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", ints.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].RootRunID != "run_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two pending sets and a cursor", first.Rows)
	}

	second, err := c.ListPendingInterruptPage(ctx, "ses_1", "", run.Capabilities{}, first.NextCursor, explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if ints.afterRunID != "run_2" {
		t.Fatalf("second page sought past %q, want the first page's last row", ints.afterRunID)
	}
	if len(second.Rows) != 1 || second.Rows[0].RootRunID != "run_3" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}
	if _, listPendingInterruptPageErr := c.ListPendingInterruptPage(ctx, "ses_1", "", run.Capabilities{}, first.NextCursor+"x", explicitPageLimit(t, 2)); !errors.Is(listPendingInterruptPageErr, pagination.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", listPendingInterruptPageErr)
	}

	// The run page is scoped and ordered the same way, so only the query namespace
	// separates the two — in both directions.
	runPage, err := c.ListRunPage(ctx, RunPageFilter{SessionID: "ses_1"}, "", explicitPageLimit(t, 2))
	if err != nil {
		t.Fatalf("run page: %v", err)
	}
	if _, err := c.ListPendingInterruptPage(ctx, "ses_1", "", run.Capabilities{}, runPage.NextCursor, explicitPageLimit(t, 2)); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("run cursor on the interrupt page err = %v, want ErrInvalidCursor", err)
	}
}
