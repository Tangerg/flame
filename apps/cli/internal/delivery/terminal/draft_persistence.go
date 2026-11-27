package terminal

import (
	"errors"
	"sync"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

var errDraftPersistenceClosed = errors.New("draft persistence is closed")

type draftRevision uint64

type draftSnapshot struct {
	revision  draftRevision
	sessionID string
	message   authoring.Message
}

type draftPersistenceResult struct {
	revision draftRevision
	err      error
}

type draftPersistence struct {
	writeDraft func(string, authoring.Message) error
	notify     func(draftPersistenceResult)

	commandMu sync.Mutex
	mu        sync.Mutex
	pending   *draftSnapshot
	revision  draftRevision
	closed    bool
	wake      chan struct{}
	commit    chan draftCommit
	shutdown  chan chan error
	done      chan struct{}
}

type draftCommit struct {
	revision draftRevision
	apply    func() error
	done     chan error
}

func newDraftPersistence(writeDraft func(string, authoring.Message) error, notify func(draftPersistenceResult)) *draftPersistence {
	persistence := &draftPersistence{
		writeDraft: writeDraft,
		notify:     notify,
		wake:       make(chan struct{}, 1),
		commit:     make(chan draftCommit),
		shutdown:   make(chan chan error),
		done:       make(chan struct{}),
	}
	go persistence.run()
	return persistence
}

// The snapshot is cloned because attachment slices remain owned by the UI model.
func (d *draftPersistence) Schedule(sessionID string, message authoring.Message) error {
	if sessionID == "" {
		return nil
	}
	d.mu.Lock()
	revision, err := d.advanceLocked()
	if err != nil {
		d.mu.Unlock()
		return err
	}
	snapshot := draftSnapshot{revision: revision, sessionID: sessionID, message: message.Clone()}
	d.pending = &snapshot
	d.mu.Unlock()
	d.signal()
	return nil
}

// A workbench change that consumes the visible draft must run after any older
// autosave has finished so that an old snapshot cannot bring the draft back.
func (d *draftPersistence) Commit(apply func() error) error {
	d.commandMu.Lock()
	defer d.commandMu.Unlock()
	d.mu.Lock()
	revision, err := d.advanceLocked()
	d.mu.Unlock()
	if err != nil {
		return err
	}
	request := draftCommit{revision: revision, apply: apply, done: make(chan error, 1)}
	select {
	case d.commit <- request:
		return <-request.done
	case <-d.done:
		return errDraftPersistenceClosed
	}
}

func (d *draftPersistence) Current(revision draftRevision) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.closed && revision == d.revision
}

func (d *draftPersistence) Close() error {
	d.commandMu.Lock()
	defer d.commandMu.Unlock()
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		<-d.done
		return nil
	}
	d.closed = true
	d.mu.Unlock()

	result := make(chan error, 1)
	select {
	case d.shutdown <- result:
		err := <-result
		<-d.done
		return err
	case <-d.done:
		return nil
	}
}

func (d *draftPersistence) advanceLocked() (draftRevision, error) {
	if d.closed {
		return 0, errDraftPersistenceClosed
	}
	d.revision++
	return d.revision, nil
}

func (d *draftPersistence) takePending() (draftSnapshot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pending == nil {
		return draftSnapshot{}, false
	}
	snapshot := *d.pending
	d.pending = nil
	return snapshot, true
}

func (d *draftPersistence) discardPendingThrough(revision draftRevision) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pending != nil && d.pending.revision <= revision {
		d.pending = nil
	}
}

func (d *draftPersistence) signal() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *draftPersistence) run() {
	defer close(d.done)
	for {
		select {
		case <-d.wake:
			if snapshot, ok := d.takePending(); ok {
				d.publish(snapshot, d.save(snapshot))
			}
		case request := <-d.commit:
			err := request.apply()
			if err == nil {
				d.discardPendingThrough(request.revision)
			}
			request.done <- err
			if snapshot, ok := d.takePending(); ok {
				d.publish(snapshot, d.save(snapshot))
			}
		case result := <-d.shutdown:
			var err error
			if snapshot, ok := d.takePending(); ok {
				err = d.save(snapshot)
			}
			result <- err
			return
		}
	}
}

func (d *draftPersistence) save(snapshot draftSnapshot) error {
	return d.writeDraft(snapshot.sessionID, snapshot.message)
}

func (d *draftPersistence) publish(snapshot draftSnapshot, err error) {
	d.notify(draftPersistenceResult{revision: snapshot.revision, err: err})
}
