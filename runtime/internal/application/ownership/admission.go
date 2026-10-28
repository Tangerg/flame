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
	mu            sync.Mutex
	runs          map[string]reservedRun
	pending       map[*runAdmissionLease]reservedRun
	maintenance   map[*maintenanceReservation]struct{}
	claims        map[string]sessionClaimKind
	treeMutations map[string]struct{}
	changed       chan struct{}
	resolveTree   func(string) (string, error)
}

// NewGate uses the workspace owner's physical identity for both Run admission
// and destructive mutations, so aliases cannot create independent reservations.
func NewGate(resolveTree func(string) (string, error)) (*Gate, error) {
	if resolveTree == nil {
		return nil, errors.New("session admission: working-tree resolver is required")
	}
	return &Gate{
		resolveTree:   resolveTree,
		runs:          make(map[string]reservedRun),
		pending:       make(map[*runAdmissionLease]reservedRun),
		maintenance:   make(map[*maintenanceReservation]struct{}),
		claims:        make(map[string]sessionClaimKind),
		treeMutations: make(map[string]struct{}),
		changed:       make(chan struct{}),
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
	once sync.Once
}

type sessionClaimKind uint8

const (
	sessionMutation sessionClaimKind = iota
	sessionRecovery
)

// Admit converts the pending reservation into the live run identified by
// runID. It returns false when the reservation had already been released or
// admitted, or when runID is empty.
func (r RunAdmission) Admit(runID string) bool {
	if r.lease == nil {
		return false
	}
	if err := identity.ValidateRun(runID); err != nil {
		return false
	}
	admitted := false
	r.lease.once.Do(func() {
		g := r.lease.gate
		g.mu.Lock()
		defer g.mu.Unlock()
		pending, ok := g.pending[r.lease]
		if !ok {
			return
		}
		delete(g.pending, r.lease)
		g.runs[runID] = pending
		admitted = true
	})
	return admitted
}

// Release abandons a pending run reservation. It does nothing after Admit.
func (r RunAdmission) Release() {
	if r.lease == nil {
		return
	}
	r.lease.once.Do(func() {
		g := r.lease.gate
		g.mu.Lock()
		_, ok := g.pending[r.lease]
		if !ok {
			g.mu.Unlock()
			return
		}
		delete(g.pending, r.lease)
		g.notifyLocked()
		g.mu.Unlock()
	})
}

// AcquireSession reserves one session's single-writer slot. Release is safe to
// call more than once and affects only this acquisition.
func (g *Gate) AcquireSession(ctx context.Context, sessionID string) (release func(), ok bool, err error) {
	if err := g.lockForegroundSession(ctx, sessionID); err != nil {
		return nil, false, err
	}
	defer g.mu.Unlock()
	return g.acquireSessionLocked(sessionID, sessionMutation)
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
	defer g.mu.Unlock()
	if g.activeSessionLocked(sessionID) {
		return RunAdmission{}, false, nil
	}
	if cwd != "" {
		if _, busy := g.treeMutations[cwd]; busy {
			return RunAdmission{}, false, nil
		}
	}
	admission := &runAdmissionLease{gate: g}
	g.pending[admission] = reservedRun{sessionID: sessionID, cwd: cwd}
	return RunAdmission{lease: admission}, true, nil
}

// BeginMaintenance converts a live run into a maintenance reservation. Both
// its session and working tree remain unavailable until Release returns, so a
// checkpoint snapshot cannot race a destructive mutation of the same tree.
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
// live, or executing synchronous terminal maintenance on that working tree.
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
	if _, busy := g.treeMutations[cwd]; busy || g.treeReservedLocked(cwd) {
		return nil, false, nil
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
	for _, pending := range g.pending {
		if !yield(pending.sessionID) {
			return
		}
	}
	for _, run := range g.runs {
		if !yield(run.sessionID) {
			return
		}
	}
	for reservation := range g.maintenance {
		if !yield(reservation.run.sessionID) {
			return
		}
	}
}

// WaitRunStartable blocks until sessionID has no pending, live, maintenance, or
// session-only admission and cwd has no destructive working-tree mutation. It
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
		_, treeMutation := g.treeMutations[cwd]
		if !g.activeSessionLocked(sessionID) && (cwd == "" || !treeMutation) {
			g.mu.Unlock()
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

// treeReservedLocked reports whether a pending, live, or maintenance Run holds
// cwd.
func (g *Gate) treeReservedLocked(cwd string) bool {
	for _, run := range g.pending {
		if run.cwd == cwd {
			return true
		}
	}
	for _, run := range g.runs {
		if run.cwd == cwd {
			return true
		}
	}
	for reservation := range g.maintenance {
		if reservation.run.cwd == cwd {
			return true
		}
	}
	return false
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
