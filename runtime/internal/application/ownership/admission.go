package ownership

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// Gate serializes one writer per session, records live Runs, and coordinates
// their working-tree admissions with destructive workspace mutations. Construct
// it with NewGate so every admission uses the ownership backend.
type Gate struct {
	mu            sync.Mutex
	runs          map[string]liveRun
	pending       map[*runAdmissionLease]pendingRun
	claims        map[string]map[*sessionClaim]struct{}
	treeRuns      map[string]int
	treeMutations map[string]struct{}
	changed       chan struct{}
	ownership     AdmissionBackend
}

// AdmissionBackend maps product identities to non-blocking cross-process leases.
// Acquired=false with no error means contention. Operational failures must
// return their cause so callers never present them as ordinary contention.
type AdmissionBackend interface {
	TrySession(sessionID string) (Lease, bool, error)
	TryWorkingTree(cwd string, shared bool) (Lease, bool, error)
}

// NewGate constructs a Gate whose single-writer and working-tree invariants span
// every Runtime process sharing the required ownership backend.
func NewGate(ownership AdmissionBackend) (*Gate, error) {
	if nilDependency(ownership) {
		return nil, errors.New("session admission: ownership backend is required")
	}
	return &Gate{
		ownership:     ownership,
		runs:          make(map[string]liveRun),
		pending:       make(map[*runAdmissionLease]pendingRun),
		claims:        make(map[string]map[*sessionClaim]struct{}),
		treeRuns:      make(map[string]int),
		treeMutations: make(map[string]struct{}),
		changed:       make(chan struct{}),
	}, nil
}

type liveRun struct {
	sessionID string
	cwd       string
	leases    []Lease
}

type pendingRun struct {
	sessionID string
	cwd       string
	leases    []Lease
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

// sessionClaim is one process-local ownership registration. Its address is the
// registry identity because the claim never crosses the Gate boundary.
type sessionClaim struct {
	sessionID string
}

// Admit converts the pending reservation into the live run identified by
// runID. It returns false when the reservation had already been released or
// admitted, or when runID is empty.
func (r RunAdmission) Admit(runID string) bool {
	if r.lease == nil {
		return false
	}
	if _, err := resourceid.ParseRun(runID); err != nil {
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
		g.releaseTreeRunLocked(pending.cwd)
		g.runs[runID] = liveRun(pending)
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
		pending, ok := g.pending[r.lease]
		if !ok {
			g.mu.Unlock()
			return
		}
		delete(g.pending, r.lease)
		g.releaseTreeRunLocked(pending.cwd)
		g.notifyLocked()
		g.mu.Unlock()
		releaseLeases(pending.leases)
	})
}

// AcquireSession reserves one session's single-writer slot. Release is safe to
// call more than once and affects only this acquisition.
func (g *Gate) AcquireSession(sessionID string) (release func(), ok bool, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activeSessionLocked(sessionID) {
		return nil, false, nil
	}
	lease, ok, err := g.ownership.TrySession(sessionID)
	if err != nil || !ok {
		return nil, false, err
	}
	releaseLocal := g.addClaimLocked(sessionID)
	var once sync.Once
	return func() {
		once.Do(func() {
			releaseLocal()
			lease.Release()
		})
	}, true, nil
}

// AcquireRun atomically reserves a fresh run's session and working tree. The
// returned admission must be either admitted after the durable opening commit
// or released when admission fails.
func (g *Gate) AcquireRun(sessionID, cwd string) (RunAdmission, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.activeSessionLocked(sessionID) {
		return RunAdmission{}, false, nil
	}
	sessionLease, ok, err := g.ownership.TrySession(sessionID)
	if err != nil || !ok {
		return RunAdmission{}, false, err
	}
	leases := []Lease{sessionLease}
	if cwd != "" {
		if _, busy := g.treeMutations[cwd]; busy {
			releaseLeases(leases)
			return RunAdmission{}, false, nil
		}
		treeLease, acquired, err := g.ownership.TryWorkingTree(cwd, true)
		if err != nil || !acquired {
			releaseLeases(leases)
			return RunAdmission{}, false, err
		}
		leases = append(leases, treeLease)
		g.addTreeRunLocked(cwd)
	}
	admission := &runAdmissionLease{gate: g}
	g.pending[admission] = pendingRun{sessionID: sessionID, cwd: cwd, leases: leases}
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
	releaseSession := g.addClaimLocked(run.sessionID)
	releaseTree := g.addTreeRunLocked(run.cwd)
	var once sync.Once
	return func() {
		once.Do(func() {
			releaseTree()
			releaseSession()
			releaseLeases(run.leases)
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
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, busy := g.treeMutations[cwd]; busy || g.treeRuns[cwd] > 0 || g.hasLiveRunOnTreeLocked(cwd) {
		return nil, false, nil
	}
	lease, ok, err := g.ownership.TryWorkingTree(cwd, false)
	if err != nil || !ok {
		return nil, false, err
	}
	g.treeMutations[cwd] = struct{}{}
	releaseLocal := g.releaseTreeMutation(cwd)
	var once sync.Once
	return func() {
		once.Do(func() {
			releaseLocal()
			lease.Release()
		})
	}, true, nil
}

func releaseLeases(leases []Lease) {
	for index := len(leases) - 1; index >= 0; index-- {
		leases[index].Release()
	}
}

// ActiveSessions snapshots every session with a pending or live Run, or a held
// session-only admission.
func (g *Gate) ActiveSessions() map[string]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	set := make(map[string]bool, len(g.runs)+len(g.pending)+len(g.claims))
	for id := range g.claims {
		set[id] = true
	}
	for _, pending := range g.pending {
		set[pending.sessionID] = true
	}
	for _, run := range g.runs {
		set[run.sessionID] = true
	}
	return set
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
	for {
		g.mu.Lock()
		_, treeMutation := g.treeMutations[cwd]
		if !g.activeSessionLocked(sessionID) && (cwd == "" || !treeMutation) {
			g.mu.Unlock()
			return nil
		}
		changed := g.changed
		g.mu.Unlock()

		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-changed:
		}
	}
}

func (g *Gate) activeSessionLocked(sessionID string) bool {
	if len(g.claims[sessionID]) > 0 {
		return true
	}
	for _, pending := range g.pending {
		if pending.sessionID == sessionID {
			return true
		}
	}
	for _, run := range g.runs {
		if run.sessionID == sessionID {
			return true
		}
	}
	return false
}

func (g *Gate) hasLiveRunOnTreeLocked(cwd string) bool {
	for _, run := range g.runs {
		if run.cwd == cwd {
			return true
		}
	}
	return false
}

func (g *Gate) addClaimLocked(sessionID string) func() {
	claim := &sessionClaim{sessionID: sessionID}
	owners := g.claims[sessionID]
	if owners == nil {
		owners = map[*sessionClaim]struct{}{}
		g.claims[sessionID] = owners
	}
	owners[claim] = struct{}{}

	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			owners := g.claims[claim.sessionID]
			delete(owners, claim)
			if len(owners) == 0 {
				delete(g.claims, claim.sessionID)
			}
			g.notifyLocked()
		})
	}
}

func (g *Gate) notifyLocked() {
	close(g.changed)
	g.changed = make(chan struct{})
}

func (g *Gate) addTreeRunLocked(cwd string) func() {
	if cwd == "" {
		return func() {}
	}
	g.treeRuns[cwd]++
	return g.releaseTreeRun(cwd)
}

func (g *Gate) releaseTreeRunLocked(cwd string) {
	if cwd == "" {
		return
	}
	if g.treeRuns[cwd] <= 1 {
		delete(g.treeRuns, cwd)
		return
	}
	g.treeRuns[cwd]--
}

func (g *Gate) releaseTreeRun(cwd string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.releaseTreeRunLocked(cwd)
		})
	}
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
