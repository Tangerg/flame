package ownership

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

type Gate struct {
	mu               sync.Mutex
	runs             map[identity.RunID]reservedRun
	pending          map[*runAdmissionLease]reservedRun
	maintenance      map[*maintenanceReservation]struct{}
	claims           map[identity.SessionID]sessionClaimKind
	treeMutations    map[string]struct{}
	changed          chan struct{}
	resolveTree      func(string) (string, error)
	treesOverlap     func(string, string) (bool, error)
	settleRetirement func(context.Context, identity.SessionID) (bool, error)
	rollbackPending  func(context.Context, identity.SessionID) (bool, error)
}

type GateConfig struct {
	ResolveTree              func(string) (string, error)
	TreesOverlap             func(string, string) (bool, error)
	SettleRetirement         func(context.Context, identity.SessionID) (bool, error)
	WorkspaceRollbackPending func(context.Context, identity.SessionID) (bool, error)
}

func NewGate(cfg GateConfig) (*Gate, error) {
	if cfg.ResolveTree == nil {
		return nil, errors.New("session admission: working-tree resolver is required")
	}
	if cfg.TreesOverlap == nil {
		return nil, errors.New("session admission: working-tree comparison is required")
	}
	if cfg.SettleRetirement == nil {
		return nil, errors.New("session admission: resource retirement settlement is required")
	}
	if cfg.WorkspaceRollbackPending == nil {
		return nil, errors.New("session admission: workspace rollback fence is required")
	}
	return &Gate{
		resolveTree:      cfg.ResolveTree,
		treesOverlap:     cfg.TreesOverlap,
		settleRetirement: cfg.SettleRetirement,
		rollbackPending:  cfg.WorkspaceRollbackPending,
		runs:             make(map[identity.RunID]reservedRun),
		pending:          make(map[*runAdmissionLease]reservedRun),
		maintenance:      make(map[*maintenanceReservation]struct{}),
		claims:           make(map[identity.SessionID]sessionClaimKind),
		treeMutations:    make(map[string]struct{}),
		changed:          make(chan struct{}),
	}, nil
}

type reservedRun struct {
	sessionID identity.SessionID
	cwd       string
}

type maintenanceReservation struct {
	run reservedRun
}

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

func (r RunAdmission) Admit(runID identity.RunID) bool {
	if r.lease == nil {
		return false
	}
	if err := runID.Validate(); err != nil {
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

func (g *Gate) AcquireSession(ctx context.Context, sessionID identity.SessionID) (release func(), ok bool, err error) {
	release, ok, err = g.acquireForegroundSession(ctx, sessionID)
	if err != nil || !ok {
		return nil, false, err
	}
	return g.checkRollbackFence(ctx, sessionID, release)
}

func (g *Gate) AcquireWorkspaceRollback(ctx context.Context, sessionID identity.SessionID) (release func(), ok bool, err error) {
	return g.acquireForegroundSession(ctx, sessionID)
}

func (g *Gate) acquireForegroundSession(ctx context.Context, sessionID identity.SessionID) (release func(), ok bool, err error) {
	if err := g.lockForegroundSession(ctx, sessionID); err != nil {
		return nil, false, err
	}
	release, ok, err = g.acquireSessionLocked(sessionID, sessionMutation)
	g.mu.Unlock()
	if err != nil || !ok {
		return nil, false, err
	}
	settled, err := g.settleRetirement(ctx, sessionID)
	err = errors.Join(err, errors.Join(ctx.Err(), context.Cause(ctx)))
	if err != nil || !settled {
		release()
		return nil, false, err
	}
	return release, true, nil
}

func (g *Gate) AcquireRecoverySession(ctx context.Context, sessionID identity.SessionID) (release func(), ok bool, err error) {
	g.mu.Lock()
	release, ok, err = g.acquireSessionLocked(sessionID, sessionRecovery)
	g.mu.Unlock()
	if err != nil || !ok {
		return nil, false, err
	}
	return g.checkRollbackFence(ctx, sessionID, release)
}

func (g *Gate) checkRollbackFence(ctx context.Context, sessionID identity.SessionID, release func()) (func(), bool, error) {
	pending, err := g.rollbackPending(ctx, sessionID)
	if err != nil || pending {
		release()
		return nil, false, err
	}
	return release, true, nil
}

func (g *Gate) acquireSessionLocked(sessionID identity.SessionID, kind sessionClaimKind) (release func(), ok bool, err error) {
	if sessionID.IsZero() {
		return nil, false, errors.New("session admission: session identity is required")
	}
	if g.activeSessionLocked(sessionID) {
		return nil, false, nil
	}
	return g.addClaimLocked(sessionID, kind), true, nil
}

func (g *Gate) AcquireRun(ctx context.Context, sessionID identity.SessionID, cwd string) (RunAdmission, bool, error) {
	if sessionID.IsZero() {
		return RunAdmission{}, false, errors.New("session admission: session identity is required")
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
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
	settled, err := g.settleRetirement(ctx, sessionID)
	err = errors.Join(err, errors.Join(ctx.Err(), context.Cause(ctx)))
	if err != nil || !settled {
		admission.Release()
		return RunAdmission{}, false, err
	}
	return admission, true, nil
}

func (g *Gate) acquireRunLocked(sessionID identity.SessionID, cwd string) (RunAdmission, bool, error) {
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

func (g *Gate) BeginMaintenance(runID identity.RunID) (release func(), ok bool) {
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

func (g *Gate) ActiveSessions() map[identity.SessionID]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	set := make(map[identity.SessionID]bool, len(g.runs)+len(g.pending)+len(g.maintenance)+len(g.claims))
	for id := range g.activeSessionsLocked {
		set[id] = true
	}
	return set
}

func (g *Gate) Changes() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.changed
}

// Admission and observers both read this so a session cannot be busy for one
// question and idle for the other.
func (g *Gate) activeSessionsLocked(yield func(identity.SessionID) bool) {
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

// An observation boundary, not a reservation: callers must still acquire their own
// admission and may wait again.
func (g *Gate) WaitRunStartable(ctx context.Context, sessionID identity.SessionID, cwd string) error {
	if ctx == nil {
		return errors.New("session admission: wait context is required")
	}
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
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
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
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
			return nil
		}

		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), context.Cause(ctx))
		case <-changed:
		}
	}
}

// Successful return transfers the locked Gate to the foreground admission. The
// recheck and reservation share that lock, so another probe cannot slip between them.
func (g *Gate) lockForegroundSession(ctx context.Context, sessionID identity.SessionID) error {
	for {
		g.mu.Lock()
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
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
			return errors.Join(ctx.Err(), context.Cause(ctx))
		case <-changed:
		}
	}
}

func (g *Gate) activeSessionLocked(sessionID identity.SessionID) bool {
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

func (g *Gate) addClaimLocked(sessionID identity.SessionID, kind sessionClaimKind) func() {
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
