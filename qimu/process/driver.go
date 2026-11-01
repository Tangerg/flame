// Package process binds Qimu's approved execution to a durable local runner
// and a consumer-selected stream protocol. It never selects a provider.
package process

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	"github.com/Tangerg/qimu"
	"github.com/Tangerg/qimu/integrity"
	"github.com/Tangerg/qimu/jsonvalue"
	"github.com/Tangerg/qimu/runner"
	"github.com/Tangerg/qimu/sqlite"
)

// Program is an immutable distribution projection. Secret material belongs in
// Env, never Args or Configuration; CredentialRevision is an opaque revision.
type Program struct {
	Command            string
	ExecutableDigest   integrity.Digest
	Args               []string
	Env                []string
	Directory          string
	CredentialRevision string
	Metadata           map[string]jsonvalue.Value
	// Exports maps logical declarations to physical exports. It cannot
	// change their contracts or choose a provider. Omitted entries keep the name.
	Exports map[string]string
	// ReleaseContent transfers an acquired immutable-content lease to the instance.
	ReleaseContent func() error
}

type Source interface {
	Program(context.Context, qimu.UnitBinding) (Program, error)
}

// Runner is the original process authority; a consumer may start and connect its
// independent daemon before delegating these protocol operations.
type Runner interface {
	Identity(context.Context) (runner.Identity, error)
	Fence(context.Context, uint64) error
	Launch(context.Context, runner.LaunchRequest) (runner.Observation, error)
	Attach(context.Context, string) (io.ReadWriteCloser, error)
	Inspect(context.Context, string) (runner.Observation, error)
	CloseAttempt(context.Context, string) (runner.Observation, error)
	AcknowledgeExit(context.Context, runner.ExitReceipt) error
	ProveAbsent(context.Context, runner.AbsenceRequest) (runner.AbsenceProof, error)
}

type Config struct {
	Runner       Runner
	Store        *sqlite.Store
	Source       Source
	CloseTimeout time.Duration
}

// Bind validates the captured contract before launching anything. A partially
// connected session remains owned by the driver even if setup returns an error.
type Protocol[S io.Closer] interface {
	Name() qimu.Profile
	Bind(*qimu.Setup, Program) (Connect[S], error)
}
type Connect[S io.Closer] func(context.Context, io.ReadWriteCloser) (S, []qimu.ExportValue, error)

type Driver[S io.Closer] struct {
	profile Protocol[S]
	config  Config
	mu      sync.Mutex
	live    map[string]*execution[S]
}

type execution[S io.Closer] struct {
	channel  io.Closer
	driver   *Driver[S]
	attempt  sqlite.Attempt
	ready    chan struct{}
	mu       sync.Mutex
	session  S
	started  bool
	resource *qimu.ResourceHandle
	cancel   context.CancelCauseFunc
}

// A protocol and the driver may both close the attached stream. They share its
// single close result instead of closing one underlying socket twice.
type transport struct {
	io.ReadWriteCloser
	once sync.Once
	err  error
}

func (t *transport) Close() error {
	t.once.Do(func() { t.err = t.ReadWriteCloser.Close() })
	return t.err
}

func New[S io.Closer](config Config, profile Protocol[S]) (*Driver[S], error) {
	if nilDependency(config.Runner) || config.Store == nil || nilDependency(config.Source) || config.CloseTimeout <= 0 || nilDependency(profile) || profile.Name().Validate() != nil {
		return nil, fmt.Errorf("%w: process driver requires runner, store, source and explicit limits", qimu.ErrInvalid)
	}
	return &Driver[S]{profile: profile, config: config, live: make(map[string]*execution[S])}, nil
}

func nilDependency(dependency any) bool {
	value := reflect.ValueOf(dependency)
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return value.IsNil()
	default:
		return false
	}
}

func (d *Driver[S]) Profile() qimu.Profile { return d.profile.Name() }

func (d *Driver[S]) Prepare(ctx context.Context, setup *qimu.Setup) ([]qimu.ExportValue, error) {
	if setup == nil || setup.Epoch() != d.config.Store.Epoch() {
		return nil, qimu.ErrFenced
	}
	unit := setup.Unit()
	for _, requirement := range unit.Imports {
		if requirement.Contract.Mode != qimu.PortableMode {
			return nil, qimu.ErrContractMismatch
		}
	}
	for _, export := range unit.Exports {
		if export.Contract.Mode != qimu.PortableMode {
			return nil, qimu.ErrContractMismatch
		}
	}
	program, err := d.config.Source.Program(ctx, setup.Binding())
	var content *qimu.ResourceHandle
	if program.ReleaseContent != nil {
		var ownErr error
		content, ownErr = setup.Cleanup("process content", program.ReleaseContent)
		if ownErr != nil {
			return nil, errors.Join(err, ownErr, program.ReleaseContent())
		}
	}
	if err != nil {
		return nil, err
	}
	connect, err := d.profile.Bind(setup, program)
	if err != nil {
		return nil, err
	}
	if connect == nil {
		return nil, qimu.ErrInvalid
	}
	if err := d.config.Runner.Fence(ctx, setup.Epoch()); err != nil {
		return nil, err
	}
	identity, err := d.config.Runner.Identity(ctx)
	if err != nil {
		return nil, err
	}
	if identity.Fenced || identity.Epoch != setup.Epoch() {
		return nil, qimu.ErrFenced
	}
	attempt := sqlite.Attempt{ID: setup.InstanceID(), Epoch: setup.Epoch(), Source: setup.Binding().Source, Binding: setup.Snapshot(), Realm: identity.ID, Profile: d.Profile()}
	if err := d.config.Store.RecordAttempt(ctx, attempt); err != nil {
		return nil, err
	}
	ownedCtx, cancel := context.WithCancelCause(setup.Lifetime().Context())
	stopPreparation := context.AfterFunc(ctx, func() { cancel(context.Cause(ctx)) })
	defer stopPreparation()
	e := &execution[S]{driver: d, attempt: attempt, ready: make(chan struct{}), cancel: cancel}
	var resources []*qimu.ResourceHandle
	if content != nil {
		resources = append(resources, content)
	}
	resource, err := setup.Own(qimu.Resource{Name: "process " + attempt.ID, Cancel: cancel, Close: e.close}, resources...)
	if err != nil {
		cancel(err)
		// No creation was dispatched; this proof is local, exact and final.
		proof := sqlite.ExitProof{AttemptID: attempt.ID, Realm: attempt.Realm, ExitID: attempt.ID + ":not-dispatched", Kind: "not-dispatched", Evidence: []byte(`{"dispatched":false}`)}
		return nil, errors.Join(err, d.config.Store.ReleaseAttempt(context.WithoutCancel(ctx), attempt.ID, proof))
	}
	e.resource = resource
	d.mu.Lock()
	d.live[attempt.ID] = e
	d.mu.Unlock()
	defer close(e.ready)
	if err := ownedCtx.Err(); err != nil {
		return nil, errors.Join(err, context.Cause(ownedCtx))
	}
	e.mu.Lock()
	e.started = true
	e.mu.Unlock()
	observation, err := d.config.Runner.Launch(ownedCtx, runner.LaunchRequest{
		AttemptID: attempt.ID, Epoch: attempt.Epoch, Command: program.Command,
		ExecutableDigest: program.ExecutableDigest, Args: program.Args, Env: program.Env,
		Directory: program.Directory, Configuration: setup.Binding().Configuration,
		CredentialRevision: program.CredentialRevision,
	})
	if err != nil {
		return nil, err
	}
	if observation.RunnerID != attempt.Realm || observation.State != runner.Running {
		return nil, runner.ErrUnknown
	}
	attached, err := d.config.Runner.Attach(ownedCtx, attempt.ID)
	if err != nil {
		return nil, err
	}
	channel := &transport{ReadWriteCloser: attached}
	e.mu.Lock()
	e.channel = channel
	e.mu.Unlock()
	stopConnect := context.AfterFunc(ownedCtx, func() { _ = channel.Close() })
	defer stopConnect()
	session, values, err := connect(ownedCtx, channel)
	e.mu.Lock()
	e.session = session
	e.mu.Unlock()
	if err == nil && nilDependency(session) {
		return nil, qimu.ErrInvalid
	}
	return values, err
}

// SessionBorrow holds actual work on the approved Use until Close. A consumer
// must not retain Session or start new calls after closing this borrow.
type SessionBorrow[S io.Closer] struct {
	work    *qimu.Work
	session S
}

func (b *SessionBorrow[S]) Context() context.Context { return b.work.Context() }
func (b *SessionBorrow[S]) Session() S               { return b.session }
func (b *SessionBorrow[S]) Close()                   { b.work.Finish() }

func (d *Driver[S]) BorrowSession(ctx context.Context, use *qimu.Use, unit qimu.UnitRef) (*SessionBorrow[S], error) {
	if use == nil {
		return nil, qimu.ErrInvalid
	}
	work, err := use.Lifetime().Enter(ctx)
	if err != nil {
		return nil, err
	}
	borrow := &SessionBorrow[S]{work: work}
	err = qimu.BorrowInstance(work.Context(), use, unit, func(ctx context.Context, instanceID string) error {
		d.mu.Lock()
		execution := d.live[instanceID]
		d.mu.Unlock()
		if execution == nil {
			return qimu.ErrUnavailable
		}
		execution.mu.Lock()
		borrow.session = execution.session
		execution.mu.Unlock()
		if nilDependency(borrow.session) {
			return qimu.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		work.Finish()
		return nil, err
	}
	return borrow, nil
}

func (e *execution[S]) close(ctx context.Context) (qimu.ReleaseState, error) {
	e.cancel(qimu.ErrClosed)
	ctx, cancel := context.WithTimeout(ctx, e.driver.config.CloseTimeout)
	defer cancel()
	select {
	case <-e.ready:
	case <-ctx.Done():
		return qimu.ResourceUnconfirmed, context.Cause(ctx)
	}
	e.mu.Lock()
	session, channel, started := e.session, e.channel, e.started
	var empty S
	e.session, e.channel = empty, nil
	e.mu.Unlock()
	var transportErr error
	if !nilDependency(session) {
		transportErr = session.Close()
	}
	if channel != nil {
		transportErr = errors.Join(transportErr, channel.Close())
	}
	if !started {
		proof := sqlite.ExitProof{AttemptID: e.attempt.ID, Realm: e.attempt.Realm, ExitID: e.attempt.ID + ":not-dispatched", Kind: "not-dispatched", Evidence: []byte(`{"dispatched":false}`)}
		if err := e.driver.config.Store.ReleaseAttempt(ctx, e.attempt.ID, proof); err != nil {
			return qimu.ResourceUnconfirmed, errors.Join(transportErr, err)
		}
		e.driver.forget(e.attempt.ID)
		return qimu.ResourceReleased, transportErr
	}
	observed, err := e.driver.config.Runner.CloseAttempt(ctx, e.attempt.ID)
	if err != nil {
		return qimu.ResourceUnconfirmed, errors.Join(transportErr, err)
	}
	if err := e.driver.release(ctx, e.attempt, observed); err != nil {
		return qimu.ResourceUnconfirmed, errors.Join(transportErr, err)
	}
	e.driver.forget(e.attempt.ID)
	return qimu.ResourceReleased, transportErr
}

func (d *Driver[S]) forget(id string) {
	d.mu.Lock()
	delete(d.live, id)
	d.mu.Unlock()
}

func (d *Driver[S]) release(ctx context.Context, attempt sqlite.Attempt, observed runner.Observation) error {
	if observed.AttemptID != attempt.ID || observed.Epoch != attempt.Epoch || observed.RunnerID != attempt.Realm || observed.State != runner.Exited || observed.ProofID == "" {
		return runner.ErrUnknown
	}
	evidence, err := json.Marshal(observed)
	if err != nil {
		return err
	}
	proof := sqlite.ExitProof{AttemptID: attempt.ID, Realm: attempt.Realm, ExitID: observed.ProofID, Kind: "runner-wait", Evidence: evidence}
	if err := d.config.Store.AcceptExit(ctx, attempt.ID, proof); err != nil {
		return err
	}
	return d.acknowledgeExit(ctx, attempt, proof)
}

func (d *Driver[S]) acknowledgeExit(ctx context.Context, attempt sqlite.Attempt, proof sqlite.ExitProof) error {
	receipt := runner.ExitReceipt{AttemptID: attempt.ID, Epoch: attempt.Epoch, RunnerID: attempt.Realm, ProofID: proof.ExitID}
	if err := d.config.Runner.AcknowledgeExit(ctx, receipt); err != nil {
		return err
	}
	return d.config.Store.ReleaseAttempt(ctx, attempt.ID, proof)
}

// Reconcile consumes late evidence for the original resource. It never repeats
// cleanup or launches a replacement under that attempt identity.
func (d *Driver[S]) Reconcile(ctx context.Context, id string) error {
	d.mu.Lock()
	e := d.live[id]
	d.mu.Unlock()
	if e == nil {
		return qimu.ErrUnavailable
	}
	observed, err := d.config.Runner.Inspect(ctx, id)
	if err != nil {
		return err
	}
	if err := d.release(ctx, e.attempt, observed); err != nil {
		return err
	}
	if err := e.resource.ConfirmReleased(nil); err != nil {
		return err
	}
	d.forget(id)
	return nil
}

// Recover seals old creation before examining absence. Only the same live
// runner can prove an old attempt did not start or has actually exited.
func (d *Driver[S]) Recover(ctx context.Context) error {
	records, err := d.config.Store.Attempts(ctx)
	if err != nil {
		return err
	}
	pending := false
	for _, record := range records {
		if (!record.Released || record.PendingHandoff) && record.Attempt.Profile == d.Profile() {
			pending = true
			break
		}
	}
	if !pending {
		return nil
	}
	identity, err := d.config.Runner.Identity(ctx)
	if err != nil {
		return err
	}
	if err := d.config.Runner.Fence(ctx, d.config.Store.Epoch()); err != nil {
		return err
	}
	var failures error
	for _, record := range records {
		if record.Attempt.Profile == d.Profile() {
			failures = errors.Join(failures, d.recoverAttempt(ctx, identity, record))
		}
	}
	return failures
}

func (d *Driver[S]) recoverAttempt(ctx context.Context, identity runner.Identity, record sqlite.AttemptRecord) error {
	attempt := record.Attempt
	if record.PendingHandoff {
		return d.acknowledgeExit(ctx, attempt, *record.Proof)
	}
	if record.Released {
		return nil
	}
	if attempt.Epoch >= d.config.Store.Epoch() {
		return runner.ErrUnknown
	}
	if attempt.Realm != identity.ID {
		// A replacement authority may replay the original durable exit but
		// cannot stop a surviving process whose wait identity it never owned.
		observed, err := d.config.Runner.Inspect(ctx, attempt.ID)
		if err == nil {
			err = d.release(ctx, attempt, observed)
		}
		if err != nil {
			return errors.Join(runner.ErrUnknown, err)
		}
		return nil
	}
	observed, err := d.config.Runner.CloseAttempt(ctx, attempt.ID)
	if errors.Is(err, runner.ErrNotFound) {
		return d.releaseAbsentAttempt(ctx, attempt)
	}
	if err != nil {
		return err
	}
	return d.release(ctx, attempt, observed)
}

func (d *Driver[S]) releaseAbsentAttempt(ctx context.Context, attempt sqlite.Attempt) error {
	request := runner.AbsenceRequest{AttemptID: attempt.ID, Epoch: attempt.Epoch, RunnerID: attempt.Realm}
	proof, err := d.config.Runner.ProveAbsent(ctx, request)
	if err == nil {
		err = proof.Validate(request)
	}
	if err != nil {
		return errors.Join(runner.ErrUnknown, err)
	}
	evidence, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	return d.config.Store.ReleaseAttempt(ctx, attempt.ID, sqlite.ExitProof{AttemptID: attempt.ID, Realm: attempt.Realm, ExitID: proof.ProofID, Kind: "sealed-not-started", Evidence: evidence})
}
