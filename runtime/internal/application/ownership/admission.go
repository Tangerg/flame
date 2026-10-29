// Package ownership owns Runtime's Run admission and ordered recovery boundaries.
package ownership

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/cancellation"
)

// Gate serializes one writer per session, records live Runs, and coordinates
// their working-tree admissions with destructive workspace mutations. Runtime's
// data-directory lease excludes other processes; this registry owns admission
// for every use case inside that lifetime.
type Gate struct {
	mu                sync.Mutex
	runs              map[string]reservedRun
	pending           map[*runAdmissionLease]reservedRun
	maintenance       map[*maintenanceReservation]struct{}
	claims            map[string]sessionClaimKind
	treeMutations     map[string]struct{}
	changed           chan struct{}
	resolveTree       func(string) (string, error)
	treesOverlap      func(string, string) (bool, error)
	retirementPending func(context.Context, string) (bool, error)
}

// GateConfig binds resolution and comparison to the same physical path semantics.
type GateConfig struct {
	ResolveTree       func(string) (string, error)
	TreesOverlap      func(string, string) (bool, error)
	RetirementPending func(context.Context, string) (bool, error)
}

// NewGate shares physical tree identity and committed resource retirement with
// every foreground admission. Recovery alone may consume a retirement fence.
func NewGate(cfg GateConfig) (*Gate, error) {
	if cfg.ResolveTree == nil {
		return nil, errors.New("session admission: working-tree resolver is required")
	}
	if cfg.TreesOverlap == nil {
		return nil, errors.New("session admission: working-tree comparison is required")
	}
	if cfg.RetirementPending == nil {
		return nil, errors.New("session admission: resource retirement reader is required")
	}
	return &Gate{
		resolveTree:       cfg.ResolveTree,
		treesOverlap:      cfg.TreesOverlap,
		retirementPending: cfg.RetirementPending,
		runs:              make(map[string]reservedRun),
		pending:           make(map[*runAdmissionLease]reservedRun),
		maintenance:       make(map[*maintenanceReservation]struct{}),
		claims:            make(map[string]sessionClaimKind),
		treeMutations:     make(map[string]struct{}),
		changed:           make(chan struct{}),
	}, nil
}

// reservedRun is one Run's hold on its session and working tree, whether it
// is pending admission, live, or in terminal maintenance; the map holding it
// says which.
type reservedRun struct {
	sessionID string
	cwd       string
}

// maintenanceReservation is a run in terminal maintenance. Its address is the
// registry identity; the field keeps it non-zero-sized so addresses are distinct.
type maintenanceReservation struct {
	run reservedRun
}

// RunAdmission owns a fresh run's session and working-tree reservation until
// it either becomes a live run or is released. Its methods are safe to call
// more than once and across value copies; only the first terminal transition
// takes effect.
type RunAdmission struct {
	lease *runAdmissionLease
}

type runAdmissionLease struct {
	gate *Gate
}

type sessionClaimKind uint8

const (
	sessionMutation sessionClaimKind = iota
	sessionRecovery
)

// A Run identity cannot transfer an existing Session reservation. A refused
// identity leaves the pending admission releasable by its original caller.
func (r RunAdmission) Admit(runID string) bool {
	if r.lease == nil {
		return false
	}
	if err := identity.ValidateRun(runID); err != nil {
		return false
	}
	g := r.lease.gate
	g.mu.Lock()
	defer g.mu.Unlock()
	pending, exists := g.pending[r.lease]
	if !exists {
		return false
	}
	if _, exists := g.runs[runID]; exists {
		return false
	}
	delete(g.pending, r.lease)
	g.runs[runID] = pending
	return true
}

// Release abandons a pending run reservation. It does nothing after Admit.
func (r RunAdmission) Release() {
	if r.lease == nil {
		return
	}
	g := r.lease.gate
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.pending[r.lease]; exists {
		delete(g.pending, r.lease)
		g.notifyLocked()
	}
}

// AcquireSession reserves one session's single-writer slot. Release is safe to
// call more than once and affects only this acquisition.
func (g *Gate) AcquireSession(ctx context.Context, sessionID string) (release func(), ok bool, err error) {
	if err := g.lockForegroundSession(ctx, sessionID); err != nil {
		return nil, false, err
	}
	release, ok, err = g.acquireSessionLocked(sessionID, sessionMutation)
	g.mu.Unlock()
	if err != nil || !ok {
		return nil, false, err
	}
	pending, err := g.retirementPending(ctx, sessionID)
	err = errors.Join(err, cancellation.Err(ctx))
	if err != nil || pending {
		release()
		return nil, false, err
	}
	return release, true, nil
}

// AcquireRecoverySession probes abandoned ownership without delaying other recovery sweeps.
// Foreground commands wait for this internal check instead of reporting a live Run conflict.
func (g *Gate) AcquireRecoverySession(sessionID string) (release func(), ok bool, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.acquireSessionLocked(sessionID, sessionRecovery)
}

func (g *Gate) acquireSessionLocked(sessionID string, kind sessionClaimKind) (release func(), ok bool, err error) {
	if sessionID == "" {
		return nil, false, errors.New("session admission: session identity is required")
	}
	if g.activeSessionLocked(sessionID) {
		return nil, false, nil
	}
	return g.addClaimLocked(sessionID, kind), true, nil
}

// AcquireRun atomically reserves a fresh run's session and working tree. The
// returned admission must be either admitted after the durable opening commit
// or released when admission fails.
func (g *Gate) AcquireRun(ctx context.Context, sessionID, cwd string) (RunAdmission, bool, error) {
	if sessionID == "" {
		return RunAdmission{}, false, errors.New("session admission: session identity is required")
	}
	if err := cancellation.Err(ctx); err != nil {
		return RunAdmission{}, false, err
	}
	if cwd != "" {
		var err error
		cwd, err = g.resolveTree(cwd)
		if err != nil {
			return RunAdmission{}, false, err
		}
	}
	if err := g.lockForegroundSession(ctx, sessionID); err != nil {
		return RunAdmission{}, false, err
	}
	admission, ok, err := g.acquireRunLocked(sessionID, cwd)
	g.mu.Unlock()
	if err != nil || !ok {
		return RunAdmission{}, false, err
	}
	pending, err := g.retirementPending(ctx, sessionID)
	err = errors.Join(err, cancellation.Err(ctx))
	if err != nil || pending {
		admission.Release()
		return RunAdmission{}, false, err
	}
	return admission, true, nil
}

func (g *Gate) acquireRunLocked(sessionID, cwd string) (RunAdmission, bool, error) {
	if g.activeSessionLocked(sessionID) {
		return RunAdmission{}, false, nil
	}
	if cwd != "" {
		if busy, err := g.treeMutationLocked(cwd); err != nil || busy {
			return RunAdmission{}, false, err
		}
	}
	admission := &runAdmissionLease{gate: g}
	g.pending[admission] = reservedRun{sessionID: sessionID, cwd: cwd}
	return RunAdmission{lease: admission}, true, nil
}

// BeginMaintenance converts a live run into a maintenance reservation. Both
// its session and working tree remain unavailable until Release returns, so a
// checkpoint snapshot cannot race a destructive mutation of an overlapping tree.
func (g *Gate) BeginMaintenance(runID string) (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	run, ok := g.runs[runID]
	if !ok {
		return nil, false
	}
	delete(g.runs, runID)
	reservation := &maintenanceReservation{run: run}
	g.maintenance[reservation] = struct{}{}
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			delete(g.maintenance, reservation)
			g.notifyLocked()
		})
	}, true
}

// AcquireWorkingTreeMutation reserves exclusive access for a destructive
// operation such as a checkpoint restore. It rejects a run while it is pending,
// live, or executing synchronous terminal maintenance in an overlapping tree.
func (g *Gate) AcquireWorkingTreeMutation(cwd string) (release func(), ok bool, err error) {
	if cwd == "" {
		return func() {}, true, nil
	}
	cwd, err = g.resolveTree(cwd)
	if err != nil {
		return nil, false, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if busy, err := g.treeMutationLocked(cwd); err != nil || busy {
		return nil, false, err
	}
	if busy, err := g.treeReservedLocked(cwd); err != nil || busy {
		return nil, false, err
	}
	g.treeMutations[cwd] = struct{}{}
	return g.releaseTreeMutation(cwd), true, nil
}

// ActiveSessions snapshots every session with a pending or live Run, or a held
// session-only admission.
func (g *Gate) ActiveSessions() map[string]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	set := make(map[string]bool, len(g.runs)+len(g.pending)+len(g.maintenance)+len(g.claims))
	for id := range g.activeSessionsLocked {
		set[id] = true
	}
	return set
}

// Changes captures the current admission version before a durable read. Its
// closure means a held claim was released; observers must re-read both facts.
func (g *Gate) Changes() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.changed
}

// activeSessionsLocked yields every session holding a session-only admission, a
// pending reservation, a live Run, or a Run in maintenance. Admission asks whether one session is
// active and observers ask which ones are; both read this, so a session cannot
// be busy for one question and idle for the other.
func (g *Gate) activeSessionsLocked(yield func(string) bool) {
	for id := range g.claims {
		if !yield(id) {
			return
		}
	}
	for run := range g.reservedRunsLocked {
		if !yield(run.sessionID) {
			return
		}
	}
}

// WaitRunStartable blocks until sessionID has no pending, live, maintenance, or
// session-only admission and cwd overlaps no destructive working-tree mutation. It
// is an observation boundary, not a reservation: callers must still acquire
// their own Run admission after it returns and may wait again if another owner
// wins that race.
func (g *Gate) WaitRunStartable(ctx context.Context, sessionID, cwd string) error {
	if ctx == nil {
		return errors.New("session admission: wait context is required")
	}
	if err := cancellation.Err(ctx); err != nil {
		return err
	}
	if cwd != "" {
		var err error
		cwd, err = g.resolveTree(cwd)
		if err != nil {
			return err
		}
	}
	for {
		g.mu.Lock()
		if err := cancellation.Err(ctx); err != nil {
			g.mu.Unlock()
			return err
		}
		treeMutation, err := g.treeMutationLocked(cwd)
		if err != nil {
			g.mu.Unlock()
			return err
		}
		startable := !g.activeSessionLocked(sessionID) && (cwd == "" || !treeMutation)
		changed := g.changed
		g.mu.Unlock()
		if startable {
			pending, err := g.retirementPending(ctx, sessionID)
			if err = errors.Join(err, cancellation.Err(ctx)); err != nil {
				return err
			}
			if !pending {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return cancellation.Err(ctx)
		case <-changed:
		}
	}
}

// Successful return transfers the locked Gate to the foreground admission. The
// recheck and reservation share that lock, so another probe cannot slip between them.
func (g *Gate) lockForegroundSession(ctx context.Context, sessionID string) error {
	for {
		g.mu.Lock()
		if err := cancellation.Err(ctx); err != nil {
			g.mu.Unlock()
			return err
		}
		if kind, held := g.claims[sessionID]; !held || kind != sessionRecovery {
			return nil
		}
		changed := g.changed
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return cancellation.Err(ctx)
		case <-changed:
		}
	}
}

func (g *Gate) activeSessionLocked(sessionID string) bool {
	for id := range g.activeSessionsLocked {
		if id == sessionID {
			return true
		}
	}
	return false
}

func (g *Gate) reservedRunsLocked(yield func(reservedRun) bool) {
	for _, run := range g.pending {
		if !yield(run) {
			return
		}
	}
	for _, run := range g.runs {
		if !yield(run) {
			return
		}
	}
	for reservation := range g.maintenance {
		if !yield(reservation.run) {
			return
		}
	}
}

func (g *Gate) treeMutationLocked(cwd string) (bool, error) {
	if cwd == "" {
		return false, nil
	}
	for tree := range g.treeMutations {
		if overlaps, err := g.treesOverlap(tree, cwd); err != nil || overlaps {
			return overlaps, err
		}
	}
	return false, nil
}

func (g *Gate) treeReservedLocked(cwd string) (bool, error) {
	for run := range g.reservedRunsLocked {
		if run.cwd == "" {
			continue
		}
		if overlaps, err := g.treesOverlap(run.cwd, cwd); err != nil || overlaps {
			return overlaps, err
		}
	}
	return false, nil
}

func (g *Gate) addClaimLocked(sessionID string, kind sessionClaimKind) func() {
	g.claims[sessionID] = kind

	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			delete(g.claims, sessionID)
			g.notifyLocked()
		})
	}
}

func (g *Gate) notifyLocked() {
	close(g.changed)
	g.changed = make(chan struct{})
}

func (g *Gate) releaseTreeMutation(cwd string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			delete(g.treeMutations, cwd)
			g.notifyLocked()
		})
	}
}
