package mcp

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/process"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// Installed projects approved Qimu instances into Flame's MCP tool catalog.
// Discovery and calls borrow Uses; catalog observations keep no execution lease.
type Installed struct {
	host     *qimu.Host
	driver   *process.Driver
	source   *ProcessSource
	registry *plugins.Registry
	owner    *qimu.Lifetime
	bindings func(context.Context) ([]plugin.Activation, error)
	ready    <-chan struct{}
	scope    qimu.ScopeGrant
	mu       sync.Mutex
}

type InstalledConfig struct {
	Host     *qimu.Host
	Driver   *process.Driver
	Source   *ProcessSource
	Registry *plugins.Registry
	Lifetime *qimu.Lifetime
	Bindings func(context.Context) ([]plugin.Activation, error)
	Ready    <-chan struct{}
}

func NewInstalled(config InstalledConfig) (*Installed, error) {
	if config.Host == nil || config.Driver == nil || config.Source == nil || config.Registry == nil || config.Lifetime == nil || config.Bindings == nil || config.Ready == nil {
		return nil, errors.New("mcp: installed catalog requires the original Qimu owners and recovery boundary")
	}
	return &Installed{host: config.Host, driver: config.Driver, source: config.Source, registry: config.Registry,
		owner: config.Lifetime, bindings: config.Bindings, ready: config.Ready}, nil
}

type installedSession struct {
	owner      *Installed
	id         mcpserver.ID
	use        *qimu.Use
	instanceID string
	session    *sdk.ClientSession
	projection processProjection
	borrow     *process.SessionBorrow
}

// A catalog observation cannot extend an instance's lifetime. Every call must
// reacquire this exact binding and prove it still names this original instance.
type installedObservation struct {
	owner      *Installed
	id         mcpserver.ID
	binding    qimu.BindingSnapshot
	instanceID string
	session    *sdk.ClientSession
	projection processProjection
}

func (s *Installed) acquire(ctx context.Context, id mcpserver.ID) (_ *installedSession, err error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.ready:
	}
	plan, err := s.registry.BackendPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	binding, err := plan.Snapshot()
	if err != nil {
		return nil, err
	}
	retained, err := s.bindings(ctx)
	if err != nil {
		return nil, err
	}
	// A fresh catalog cannot occupy a unit with a new setup before the
	// original waiting execution restores it. These graphs are read from
	// their business owners; this adapter retains no competing reference set.
	units := make(map[qimu.UnitRef]struct{}, len(binding.Units))
	for _, unit := range binding.Units {
		units[unit.Source] = struct{}{}
	}
	for _, activation := range retained {
		original, err := plugins.DecodeActivation(activation)
		if err != nil {
			return nil, err
		}
		for _, unit := range original.Units {
			if _, shared := units[unit.Source]; !shared {
				continue
			}
			if err := s.waitRetired(ctx, original); err != nil {
				return nil, err
			}
			if _, err := s.host.ActivateBinding(ctx, "", original); err != nil {
				return nil, err
			}
			break
		}
	}
	if err := s.waitRetired(ctx, binding); err != nil {
		return nil, err
	}
	if _, err := s.host.ActivatePlan(ctx, "", plan); err != nil {
		return nil, err
	}
	return s.acquireBinding(ctx, id, binding)
}

// New discovery joins an original retirement before asking for its replacement.
// Calls already using an instance never enter this wait: their nested callbacks
// must not wait for the same instance's accepted work to finish.
func (s *Installed) waitRetired(ctx context.Context, binding qimu.BindingSnapshot) error {
	for _, observed := range s.host.Observations() {
		if observed.State == qimu.Preparing || observed.State == qimu.Ready {
			continue
		}
		for _, unit := range binding.Units {
			if observed.Source.InstallationID != unit.Source.InstallationID || observed.Source.UnitID != unit.Source.UnitID {
				continue
			}
			if err := s.host.WaitInstanceRetired(ctx, observed.ID); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (s *Installed) acquireBinding(ctx context.Context, id mcpserver.ID, binding qimu.BindingSnapshot) (_ *installedSession, err error) {
	installation, found := id.Origin().Installation()
	if !found {
		return nil, plugin.ErrInvalid
	}
	var root qimu.UnitRef
	for _, candidate := range binding.UnitRoots {
		if candidate.InstallationID.String() == installation.String() && candidate.UnitID == plugins.BackendUnit(id.Name()) {
			root = candidate
			break
		}
	}
	if root.InstallationID == "" {
		return nil, qimu.ErrStaleBinding
	}
	scope, err := s.openScope(ctx)
	if err != nil {
		return nil, err
	}
	use, err := s.host.AcquireBinding(ctx, binding, qimu.UseConfig{Owner: s.owner, Scope: scope})
	if err != nil {
		return nil, err
	}
	value := &installedSession{owner: s, id: id, use: use}
	defer func() {
		if err != nil {
			if value.borrow != nil {
				value.borrow.Close()
			}
			_, closeErr := use.Close(context.WithoutCancel(ctx))
			err = errors.Join(err, closeErr)
		}
	}()
	err = qimu.BorrowInstance(ctx, use, root, func(ctx context.Context, instanceID string) error {
		value.instanceID = instanceID
		s.source.mu.Lock()
		projection, found := s.source.projections[instanceID]
		s.source.mu.Unlock()
		if !found {
			return qimu.ErrUnavailable
		}
		value.projection = projection
		return nil

	})
	if err != nil {
		return nil, err
	}
	value.borrow, err = s.driver.BorrowSession(use.Lifetime().Context(), use, root)
	if err != nil {
		return nil, err
	}
	value.session = value.borrow.Session()
	return value, nil
}

func (s *installedSession) close() error {
	ctx := context.Background()
	return errors.Join(s.release(ctx), s.owner.retire(s.instanceID))
}

func (s *installedSession) activation() (plugin.Activation, error) {
	return plugins.ProjectActivation(s.use.Binding())
}

func (s *installedSession) release(ctx context.Context) error {
	s.use.BeginClose(qimu.ErrClosed)
	s.borrow.Close()
	_, err := s.use.Close(ctx)
	return err
}

func (s *installedSession) actor() operation.Actor { return s.projection.actor }

func (s *installedSession) observe() *installedObservation {
	return &installedObservation{owner: s.owner, id: s.id, binding: s.use.Binding(), instanceID: s.instanceID, session: s.session, projection: s.projection}
}

func (s *installedObservation) activation() (plugin.Activation, error) {
	return plugins.ProjectActivation(s.binding)
}

// The Host joins actual resource exit after accepted consumers have closed.
// Waiting here would make Pool shutdown depend on the executor it precedes.
func (s *installedObservation) retire() error {
	return s.owner.retire(s.instanceID)
}

func (s *Installed) retire(instanceID string) error {
	ctx := context.Background()
	err := s.host.RetireInstance(ctx, instanceID)
	if !errors.Is(err, qimu.ErrUnavailable) {
		return err
	}
	// This identity came from an original approved instance and is never
	// reused. The Host removes it only after its resource exit is proved; an
	// installation change may finish that retirement before catalog withdrawal.
	return s.host.WaitInstanceRetired(ctx, instanceID)
}

func (s *Installed) openScope(ctx context.Context) (qimu.ScopeGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scope.ID() == "" {
		scope, err := s.host.Admission().OpenScope(ctx, "flame.mcp.catalog")
		if err != nil {
			return qimu.ScopeGrant{}, err
		}
		s.scope = scope
	}
	return s.scope, nil
}
