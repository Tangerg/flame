// Package process binds Qimu's approved execution to a durable local runner
// and the public Portable MCP protocol. It never selects a provider.
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

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/runner"
	"github.com/Tangerg/flame/qimu/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// Program is an immutable distribution projection. Secret material belongs in
// Env, never Args or Configuration; CredentialRevision is an opaque revision.
type Program struct {
	Command            string
	ExecutableDigest   qimu.Digest
	Args               []string
	Env                []string
	Directory          string
	CredentialRevision string
	Metadata           map[string]jsonvalue.Value
	// Exports maps logical declarations to physical MCP exports. It cannot
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
	ProveAbsent(context.Context, runner.AbsenceRequest) (runner.AbsenceProof, error)
}

type Config struct {
	Runner        Runner
	Store         *sqlite.Store
	Source        Source
	CloseTimeout  time.Duration
	MaxFrameBytes int
	// Configure projects a consumer's exact approved binding into MCP's setup
	// protocol. It may rename slots and attach consumer authority, but may not
	// select replacement providers. It must register acquired resources with Setup.
	Configure func(context.Context, *qimu.Setup, *sdk.ClientSession) ([]mcp.SetupContribution, sdk.ParamsBase, error)
}

type Driver struct {
	config Config
	mu     sync.Mutex
	live   map[string]*execution
}

type execution struct {
	driver   *Driver
	attempt  sqlite.Attempt
	ready    chan struct{}
	mu       sync.Mutex
	session  *sdk.ClientSession
	started  bool
	resource *qimu.ResourceHandle
	cancel   context.CancelCauseFunc
}

// MCP's reader and writer refer to the same attached stream. They share its
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

func New(config Config) (*Driver, error) {
	if nilDependency(config.Runner) || config.Store == nil || nilDependency(config.Source) || config.CloseTimeout <= 0 || config.MaxFrameBytes <= 0 {
		return nil, fmt.Errorf("%w: process driver requires runner, store, source and explicit limits", qimu.ErrInvalid)
	}
	return &Driver{config: config, live: make(map[string]*execution)}, nil
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

func (*Driver) Profile() qimu.Profile { return qimu.MCPProcess }

func (d *Driver) Prepare(ctx context.Context, setup *qimu.Setup) ([]qimu.ExportValue, error) {
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
	contracts := make(map[string]*portable.Capability, len(unit.Exports))
	required := make([]mcp.CapabilityExport, 0, len(unit.Exports))
	physicalContracts := make(map[string]*portable.Capability)
	for _, export := range unit.Exports {
		valueContract, err := setup.ValueContract(export.Contract)
		if err != nil {
			return nil, err
		}
		descriptor, err := setup.ContractDescriptor(valueContract)
		if err != nil {
			return nil, err
		}
		contract, err := portable.ParseCapability(descriptor)
		if err != nil {
			return nil, err
		}
		if contract == nil || valueContract.Mode != qimu.PortableMode || valueContract.Kind != qimu.ServiceKind || contract.Descriptor().Reference != valueContract.Name || qimu.Digest(contract.Identity().Digest()) != valueContract.Digest {
			return nil, qimu.ErrContractMismatch
		}
		contracts[export.Name] = contract
		physical := physicalExport(program, export.Name)
		if original := physicalContracts[physical]; original != nil {
			if original.Identity() != contract.Identity() {
				return nil, qimu.ErrContractMismatch
			}
		} else {
			physicalContracts[physical] = contract
			required = append(required, mcp.CapabilityExport{Name: physical, Contract: contract.Descriptor()})
		}
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
	attempt := sqlite.Attempt{ID: setup.InstanceID(), Epoch: setup.Epoch(), Source: setup.Binding().Source, Binding: setup.Snapshot(), Realm: identity.ID, Profile: qimu.MCPProcess}
	if err := d.config.Store.RecordAttempt(ctx, attempt); err != nil {
		return nil, err
	}
	ownedCtx, cancel := context.WithCancelCause(setup.Lifetime().Context())
	stopPreparation := context.AfterFunc(ctx, func() { cancel(context.Cause(ctx)) })
	defer stopPreparation()
	e := &execution{driver: d, attempt: attempt, ready: make(chan struct{}), cancel: cancel}
	var resources []*qimu.ResourceHandle
	if content != nil {
		resources = append(resources, content)
	}
	resource, err := setup.Own(qimu.Resource{Name: "mcp process " + attempt.ID, Cancel: cancel, Close: e.close}, resources...)
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
	client, err := mcp.NewProcessClient()
	if err != nil {
		return nil, errors.Join(err, channel.Close())
	}
	session, err := client.Connect(ownedCtx, &sdk.IOTransport{Reader: channel, Writer: channel, MaxLineLength: d.config.MaxFrameBytes}, nil)
	if err != nil {
		return nil, errors.Join(err, channel.Close())
	}
	e.mu.Lock()
	e.session = session
	e.mu.Unlock()
	if err := mcp.VerifyCapabilities(ownedCtx, session, required); err != nil {
		return nil, err
	}
	var metadata sdk.ParamsBase
	var contributions []mcp.SetupContribution
	if d.config.Configure != nil {
		contributions, metadata, err = d.config.Configure(ownedCtx, setup, session)
		if err != nil {
			return nil, err
		}
	} else {
		if len(unit.Imports) != 0 {
			return nil, errors.New("process: declared imports require an approved callback configuration")
		}
		for _, export := range unit.Exports {
			contributions = append(contributions, mcp.SetupContribution{ID: export.Name, Export: physicalExport(program, export.Name), Requires: []mcp.SetupDependency{}})
		}
	}
	if err := mcp.SetUp(ownedCtx, session, contributions, metadata); err != nil {
		return nil, err
	}
	if err := setup.OnActivate(func(activateCtx context.Context) error { return mcp.Activate(activateCtx, session) }); err != nil {
		return nil, err
	}
	values := make([]qimu.ExportValue, 0, len(unit.Exports))
	for _, export := range unit.Exports {
		binding, err := mcp.NewCapabilityBinding(session, physicalExport(program, export.Name), contracts[export.Name])
		if err != nil {
			return nil, err
		}
		var published any
		if value, present := program.Metadata[export.Name]; present {
			published = value
		}
		values = append(values, qimu.ExportValue{Name: export.Name, Contract: export.Contract, Metadata: published, Value: binding, Durable: true})
	}
	return values, nil
}

func physicalExport(program Program, logical string) string {
	if physical, mapped := program.Exports[logical]; mapped {
		return physical
	}
	return logical
}

// SessionBorrow holds actual work on the approved Use until Close. A consumer
// must not retain Session or start new calls after closing this borrow.
type SessionBorrow struct {
	work    *qimu.Work
	session *sdk.ClientSession
}

func (b *SessionBorrow) Context() context.Context    { return b.work.Context() }
func (b *SessionBorrow) Session() *sdk.ClientSession { return b.session }
func (b *SessionBorrow) Close()                      { b.work.Finish() }

func (d *Driver) BorrowSession(ctx context.Context, use *qimu.Use, unit qimu.UnitRef) (*SessionBorrow, error) {
	if use == nil {
		return nil, qimu.ErrInvalid
	}
	work, err := use.Lifetime().Enter(ctx)
	if err != nil {
		return nil, err
	}
	borrow := &SessionBorrow{work: work}
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
		if borrow.session == nil {
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

func (e *execution) close(ctx context.Context) (qimu.ReleaseState, error) {
	e.cancel(qimu.ErrClosed)
	ctx, cancel := context.WithTimeout(ctx, e.driver.config.CloseTimeout)
	defer cancel()
	select {
	case <-e.ready:
	case <-ctx.Done():
		return qimu.ResourceUnconfirmed, context.Cause(ctx)
	}
	e.mu.Lock()
	session, started := e.session, e.started
	e.session = nil
	e.mu.Unlock()
	var transportErr error
	if session != nil {
		transportErr = session.Close()
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

func (d *Driver) forget(id string) {
	d.mu.Lock()
	delete(d.live, id)
	d.mu.Unlock()
}

func (d *Driver) release(ctx context.Context, attempt sqlite.Attempt, observed runner.Observation) error {
	if observed.AttemptID != attempt.ID || observed.Epoch != attempt.Epoch || observed.RunnerID != attempt.Realm || observed.State != runner.Exited || observed.ProofID == "" {
		return runner.ErrUnknown
	}
	evidence, err := json.Marshal(observed)
	if err != nil {
		return err
	}
	return d.config.Store.ReleaseAttempt(ctx, attempt.ID, sqlite.ExitProof{AttemptID: attempt.ID, Realm: attempt.Realm, ExitID: observed.ProofID, Kind: "runner-wait", Evidence: evidence})
}

// Reconcile consumes late evidence for the original resource. It never repeats
// cleanup or launches a replacement under that attempt identity.
func (d *Driver) Reconcile(ctx context.Context, id string) error {
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
func (d *Driver) Recover(ctx context.Context) error {
	records, err := d.config.Store.Attempts(ctx)
	if err != nil {
		return err
	}
	pending := false
	for _, record := range records {
		if !record.Released && record.Attempt.Profile == qimu.MCPProcess {
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
		attempt := record.Attempt
		if record.Released || attempt.Profile != qimu.MCPProcess {
			continue
		}
		if attempt.Epoch >= d.config.Store.Epoch() {
			failures = errors.Join(failures, runner.ErrUnknown)
			continue
		}
		if attempt.Realm != identity.ID {
			// A restarted runner may retain an original, already durable Wait
			// proof. It may not close or infer an unowned surviving process.
			observed, inspectErr := d.config.Runner.Inspect(ctx, attempt.ID)
			if inspectErr == nil {
				inspectErr = d.release(ctx, attempt, observed)
			}
			if inspectErr != nil {
				inspectErr = errors.Join(runner.ErrUnknown, inspectErr)
			}
			failures = errors.Join(failures, inspectErr)
			continue
		}
		observed, err := d.config.Runner.CloseAttempt(ctx, attempt.ID)
		if errors.Is(err, runner.ErrNotFound) {
			request := runner.AbsenceRequest{AttemptID: attempt.ID, Epoch: attempt.Epoch, RunnerID: attempt.Realm}
			proof, proofErr := d.config.Runner.ProveAbsent(ctx, request)
			if proofErr == nil {
				proofErr = proof.Validate(request)
			}
			if proofErr != nil {
				failures = errors.Join(failures, runner.ErrUnknown, proofErr)
				continue
			}
			evidence, encodeErr := json.Marshal(proof)
			if encodeErr != nil {
				failures = errors.Join(failures, encodeErr)
				continue
			}
			err = d.config.Store.ReleaseAttempt(ctx, attempt.ID, sqlite.ExitProof{AttemptID: attempt.ID, Realm: attempt.Realm, ExitID: proof.ProofID, Kind: "sealed-not-started", Evidence: evidence})
		} else if err == nil {
			err = d.release(ctx, attempt, observed)
		}
		failures = errors.Join(failures, err)
	}
	return failures
}
