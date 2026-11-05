package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

var errBindingNotRetained = fmt.Errorf("mcp: original binding is no longer retained: %w", frame.ErrUnavailable)

// CatalogPermission authorizes reconstruction only from Flame's original
// unsettled business owners. A serialized graph cannot grant itself permission.
type CatalogPermission struct {
	admission *frame.Admission
	bindings  func(context.Context) ([]plugin.Activation, error)
	mu        sync.Mutex
	scope     frame.ScopeGrant
}

func NewCatalogPermission(admission *frame.Admission, bindings func(context.Context) ([]plugin.Activation, error)) (*CatalogPermission, error) {
	if admission == nil || bindings == nil {
		return nil, errors.New("mcp: catalog permission requires admission and original binding owners")
	}
	return &CatalogPermission{admission: admission, bindings: bindings}, nil
}

func (p *CatalogPermission) grant(ctx context.Context) (frame.ScopeGrant, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.scope.ID() == "" {
		grant, err := p.admission.OpenScope(ctx, "flame.mcp.catalog")
		if err != nil {
			return frame.ScopeGrant{}, err
		}
		p.scope = grant
	}
	return p.scope, p.admission.ValidateScope(p.scope)
}

func (p *CatalogPermission) Read(ctx context.Context, recordID string, grant frame.ScopeGrant) (frame.BindingSnapshot, error) {
	p.mu.Lock()
	accepted := p.scope.ID() != "" && grant == p.scope
	p.mu.Unlock()
	if !accepted {
		return frame.BindingSnapshot{}, frame.ErrFenced
	}
	if err := p.admission.CheckAdmitted(ctx); err != nil {
		return frame.BindingSnapshot{}, err
	}
	if err := p.admission.ValidateScope(grant); err != nil {
		return frame.BindingSnapshot{}, err
	}
	bindings, err := p.bindings(ctx)
	if err != nil {
		return frame.BindingSnapshot{}, err
	}
	for _, binding := range bindings {
		if binding.Identity.String() == recordID {
			accepted, err := plugins.DecodeActivation(binding)
			if err == nil {
				if evidence, ok := ctx.Value(restorationEvidenceKey{}).(*restorationEvidence); ok {
					evidence.observe(recordID, accepted)
				}
			}
			return accepted, err
		}
	}
	return frame.BindingSnapshot{}, errBindingNotRetained
}

// Installed projects Frame instances into Flame's MCP catalog. Observations
// retain no execution lease; each use rechecks its original plan or business row.
type Installed struct {
	host       *frame.Host
	source     *ProcessSource
	registry   *plugins.Registry
	owner      *frame.Lifetime
	permission *CatalogPermission
	ready      <-chan struct{}
}

type InstalledConfig struct {
	Host       *frame.Host
	Source     *ProcessSource
	Registry   *plugins.Registry
	Lifetime   *frame.Lifetime
	Permission *CatalogPermission
	Ready      <-chan struct{}
}

func NewInstalled(config InstalledConfig) (*Installed, error) {
	if config.Host == nil || config.Source == nil || config.Registry == nil || config.Lifetime == nil || config.Permission == nil || config.Permission.admission != config.Host.Admission() || config.Ready == nil {
		return nil, errors.New("mcp: installed catalog requires the original Frame owners and recovery boundary")
	}
	return &Installed{host: config.Host, source: config.Source, registry: config.Registry,
		owner: config.Lifetime, permission: config.Permission, ready: config.Ready}, nil
}

type installedSession struct {
	owner      *Installed
	id         mcpserver.ID
	use        *frame.Use
	plan       *frame.BindingPlan
	recordID   string
	instanceID string
	session    *sdk.ClientSession
	projection processProjection
}

type installedObservation struct {
	owner      *Installed
	id         mcpserver.ID
	binding    frame.BindingSnapshot
	plan       *frame.BindingPlan
	recordID   string
	instanceID string
	session    *sdk.ClientSession
	projection processProjection
}

func (s *Installed) acquire(ctx context.Context, id mcpserver.ID) (_ *installedSession, err error) {
	select {
	case <-ctx.Done():
		return nil, errors.Join(ctx.Err(), context.Cause(ctx))
	case <-s.ready:
	}
	plan, err := s.registry.BackendPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	binding, err := plan.Snapshot()
	if err != nil {
		return nil, plugins.FrameworkError(err)
	}
	retained, err := s.permission.bindings(ctx)
	if err != nil {
		return nil, err
	}
	units := make(map[frame.UnitRef]struct{}, len(binding.Units))
	for _, unit := range binding.Units {
		units[unit.Source] = struct{}{}
	}
	// Restore original waiting work before a fresh catalog may occupy its units.
	// Those graphs remain owned by command receipts and execution checkpoints.
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
				return nil, plugins.FrameworkError(err)
			}
			grant, err := s.permission.grant(ctx)
			if err != nil {
				return nil, err
			}
			if err := s.restoreRetained(ctx, activation, grant); err != nil {
				return nil, err
			}
			break
		}
	}
	if err := s.waitRetired(ctx, binding); err != nil {
		return nil, plugins.FrameworkError(err)
	}
	return s.acquirePlan(ctx, id, plan)
}

func (s *Installed) waitRetired(ctx context.Context, binding frame.BindingSnapshot) error {
	for _, observed := range s.host.Observations() {
		if observed.State == frame.Preparing || observed.State == frame.Ready {
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

func (s *Installed) acquirePlan(ctx context.Context, id mcpserver.ID, plan *frame.BindingPlan) (*installedSession, error) {
	grant, err := s.permission.grant(ctx)
	if err != nil {
		return nil, err
	}
	use, err := s.host.Acquire(ctx, plan, frame.UseConfig{Owner: s.owner, Scope: grant})
	if err != nil {
		return nil, plugins.FrameworkError(err)
	}
	value, err := s.projectUse(ctx, id, use)
	if err != nil {
		return nil, err
	}
	value.plan = plan
	return value, nil
}

func (s *Installed) acquireRecord(ctx context.Context, id mcpserver.ID, recordID string) (*installedSession, error) {
	grant, err := s.permission.grant(ctx)
	if err != nil {
		return nil, err
	}
	use, err := s.host.AcquireRecord(ctx, recordID, frame.UseConfig{Owner: s.owner, Scope: grant})
	if err != nil {
		return nil, plugins.FrameworkError(err)
	}
	value, err := s.projectUse(ctx, id, use)
	if err != nil {
		return nil, err
	}
	value.recordID = recordID
	return value, nil
}

func (s *installedObservation) acquire(ctx context.Context) (*installedSession, error) {
	if s.plan != nil {
		return s.owner.acquirePlan(ctx, s.id, s.plan)
	}
	return s.owner.acquireRecord(ctx, s.id, s.recordID)
}

func (s *Installed) projectUse(ctx context.Context, id mcpserver.ID, use *frame.Use) (_ *installedSession, err error) {
	defer func() {
		if err != nil {
			_, closeErr := use.Close(context.WithoutCancel(ctx))
			err = plugins.FrameworkError(errors.Join(err, closeErr))
		}
	}()
	installation, found := id.Origin().Installation()
	if !found {
		return nil, plugin.ErrInvalid
	}
	var root frame.UnitRef
	for _, candidate := range use.Binding().UnitRoots {
		if candidate.InstallationID.String() == installation.String() && candidate.UnitID == plugins.BackendUnit(id.Name()) {
			root = candidate
			break
		}
	}
	if root.InstallationID == "" {
		return nil, frame.ErrStaleBinding
	}
	work, err := use.Lifetime().Enter(ctx)
	if err != nil {
		return nil, err
	}
	defer work.Finish()
	if err := s.host.ValidateUse(work.Context(), use); err != nil {
		return nil, err
	}
	value := &installedSession{owner: s, id: id, use: use}
	s.source.mu.Lock()
	for instanceID, projection := range s.source.projections {
		if projection.source == root {
			value.instanceID, value.session, value.projection = instanceID, projection.session, projection
			break
		}
	}
	s.source.mu.Unlock()
	if value.session == nil {
		return nil, frame.ErrUnavailable
	}
	// Frame prevents a replacement of an occupied unit until the original
	// instance and all accepted Uses release. This map is only its MCP view.
	if err := s.host.ValidateUse(work.Context(), use); err != nil {
		return nil, err
	}
	return value, nil
}

func (s *installedSession) close() error {
	return errors.Join(s.release(context.Background()), s.owner.retire(s.instanceID))
}
func (s *installedSession) activation() (plugin.Activation, error) {
	return plugins.ProjectActivation(s.use.Binding())
}
func (s *installedSession) release(ctx context.Context) error { _, err := s.use.Close(ctx); return err }
func (s *installedSession) actor() operation.Actor            { return s.projection.actor }
func (s *installedSession) observe() *installedObservation {
	return &installedObservation{owner: s.owner, id: s.id, binding: s.use.Binding(), plan: s.plan, recordID: s.recordID, instanceID: s.instanceID, session: s.session, projection: s.projection}
}
func (s *installedObservation) activation() (plugin.Activation, error) {
	return plugins.ProjectActivation(s.binding)
}
func (s *installedObservation) retire() error { return s.owner.retire(s.instanceID) }

func (s *Installed) retire(instanceID string) error {
	ctx := context.Background()
	err := s.host.RetireInstance(ctx, instanceID)
	if !errors.Is(err, frame.ErrUnavailable) {
		return err
	}
	return s.host.WaitInstanceRetired(ctx, instanceID)
}
