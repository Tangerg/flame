package mcp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	connectormcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcpconnector/mcp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	app "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

type grantVault interface {
	PrepareGrant(context.Context, identity.InstallationID, mcpserver.Server, app.GrantProposal) (string, error)
	PendingGrant(context.Context, identity.InstallationID, mcpserver.ServerName, string) (app.GrantProposal, bool, error)
	RejectGrant(context.Context, identity.InstallationID, mcpserver.ServerName, string) error
	OpenGrant(context.Context, identity.InstallationID, mcpserver.ServerName, string) (mcpserver.Connection, bool, error)
	ConfirmPendingGrant(context.Context, identity.InstallationID, mcpserver.ServerName, string) error
	RetireGrants(context.Context, identity.InstallationID, mcpserver.ServerName, string) error
	ListExposure(context.Context) ([]tool.Ref, error)
	SetToolExposure(context.Context, tool.Ref, bool) error
}

type ExternalConnector struct {
	recoveryMu    sync.Mutex
	recovered     map[identity.InstallationID]string
	selected      atomic.Pointer[identity.InstallationID]
	installed     *Installed
	installations plugins.Store
	selections    plugins.SelectionStore
	vault         grantVault
	methods       connectormcp.Methods
	epoch         atomic.Pointer[string]
}

func NewExternalConnector(installed *Installed, installations plugins.Store, selections plugins.SelectionStore, vault grantVault) (*ExternalConnector, error) {
	m, err := connectormcp.NewMethods()
	if err != nil {
		return nil, err
	}
	connector := &ExternalConnector{recovered: make(map[identity.InstallationID]string), installed: installed, installations: installations, selections: selections, vault: vault, methods: m}
	connector.Invalidate()
	return connector, nil
}

type connectorUse struct {
	epoch      string
	source     plugins.ContributionBinding
	session    *installedSession
	capability *qimumcp.CapabilityBinding
}

func (c *ExternalConnector) acquire(ctx context.Context) (*connectorUse, error) {
	selection, configured, err := c.selections.Get(ctx, wire.Reference)
	if err != nil {
		return nil, err
	}
	if !configured {
		c.selected.Store(nil)
		return nil, nil
	}
	id := selection.Snapshot().Installation
	c.selected.Store(&id)
	b, found, err := plugins.SelectedContribution(ctx, c.installed.host, c.installations, c.selections, c.methods.Contract)
	if err != nil || !found {
		return nil, err
	}
	session, err := c.installed.acquire(ctx, b.ServerID)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*connectorUse, error) {
		return nil, errors.Join(err, session.release(context.Background()))
	}
	activation, err := session.activation()
	if err != nil {
		return fail(err)
	}
	if _, err := plugins.OriginalActivation([]plugin.Activation{activation}, b.InstallationID, b.Digest, b.ServerName); err != nil {
		return fail(err)
	}
	capability, err := qimumcp.ResolveCapability(ctx, session.session, b.Export, c.methods.Contract)
	if err != nil {
		return fail(err)
	}
	u := &connectorUse{epoch: *c.epoch.Load(), source: b, session: session, capability: capability}
	if err := c.recover(ctx, u); err != nil {
		return fail(err)
	}
	return u, nil
}
func connectorCall[P, R any](ctx context.Context, u *connectorUse, m *qimumcp.Method[P, R], input P) (R, error) {
	var zero R
	method, err := m.Bind(u.capability)
	if err != nil {
		return zero, err
	}
	if operation.Identity(ctx) == "" {
		ctx = operation.Start(ctx, extension.Automatic, m.Name(), "")
	}
	invocation, err := operation.Capture(ctx, portable.Target{Scope: extension.ScopeInstallation, Reference: u.source.InstallationID.String()})
	if err != nil {
		return zero, err
	}
	ctx, err = qimumcp.WithInvocation(ctx, invocation)
	if err != nil {
		return zero, err
	}
	if m.Descriptor().Kind == portable.Query {
		return method.Call(ctx, input)
	}
	return method.CallCommand(ctx, qimumcp.CommandID(operation.Identity(ctx)), input)
}
func (c *ExternalConnector) entries(ctx context.Context, u *connectorUse, name string) (wire.Directory, error) {
	return connectorCall(ctx, u, c.methods.List, wire.Selector{Name: name})
}
func (c *ExternalConnector) materialize(ctx context.Context, u *connectorUse, e wire.Entry) (mcpserver.Server, error) {
	name, err := mcpserver.ParseServerName(e.Name)
	if err != nil {
		return mcpserver.Server{}, err
	}
	connection, found, err := c.vault.OpenGrant(ctx, u.source.InstallationID, name, e.LaunchGrant)
	if err != nil {
		return mcpserver.Server{}, err
	}
	if !found {
		if err := c.reconcileGrant(ctx, u, name, e.LaunchGrant); err != nil {
			return mcpserver.Server{}, err
		}
		connection, found, err = c.vault.OpenGrant(ctx, u.source.InstallationID, name, e.LaunchGrant)
		if err != nil {
			return mcpserver.Server{}, err
		}
		if !found {
			return mcpserver.Server{}, fmt.Errorf("%w: external transport grant is unavailable", app.ErrInvalidServerConfiguration)
		}
	}
	source, err := mcpserver.ConnectorSource(u.source.InstallationID)
	if err != nil {
		return mcpserver.Server{}, err
	}
	s := mcpserver.Server{DirectoryRevision: e.Revision, Name: name, Source: source, Connection: connection, Enabled: e.Enabled, Description: e.Description}
	if e.HandshakeSeconds != 0 {
		if e.HandshakeSeconds < 0 || e.HandshakeSeconds > int64((1<<63-1)/time.Second) {
			return s, app.ErrInvalidServerConfiguration
		}
		s.HandshakeTimeout, err = mcpserver.NewHandshakeTimeout(time.Duration(e.HandshakeSeconds) * time.Second)
		if err != nil {
			return s, err
		}
	}
	return s, s.Validate()
}
func (c *ExternalConnector) List(ctx context.Context) ([]mcpserver.Server, error) {
	u, err := c.acquire(ctx)
	if err != nil || u == nil {
		return nil, err
	}
	defer u.session.release(context.Background())
	d, err := c.entries(ctx, u, "")
	if err != nil {
		return nil, err
	}
	servers := make([]mcpserver.Server, 0, len(d.Entries))
	for _, e := range d.Entries {
		s, err := c.materialize(ctx, u, e)
		if err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, nil
}
func (c *ExternalConnector) Get(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	u, err := c.acquire(ctx)
	if err != nil || u == nil {
		return mcpserver.Server{}, false, err
	}
	defer u.session.release(context.Background())
	e, ok, err := c.find(ctx, u, name)
	if err != nil || !ok {
		return mcpserver.Server{}, false, err
	}
	s, err := c.materialize(ctx, u, e)
	return s, err == nil, err
}
func (c *ExternalConnector) find(ctx context.Context, u *connectorUse, name mcpserver.ServerName) (wire.Entry, bool, error) {
	d, err := c.entries(ctx, u, name.String())
	if err != nil || len(d.Entries) == 0 {
		return wire.Entry{}, false, err
	}
	if len(d.Entries) != 1 || d.Entries[0].Name != name.String() {
		return wire.Entry{}, false, app.ErrInvalidServerConfiguration
	}
	return d.Entries[0], true, nil
}
func (c *ExternalConnector) Save(ctx context.Context, s mcpserver.Server) error {
	return c.change(ctx, s, &s)
}
func (c *ExternalConnector) Remove(ctx context.Context, current mcpserver.Server) error {
	return c.change(ctx, current, nil)
}
func (c *ExternalConnector) change(ctx context.Context, current mcpserver.Server, s *mcpserver.Server) error {
	name := current.Name
	u, err := c.acquire(ctx)
	if err != nil {
		return err
	}
	if u == nil {
		return plugin.ErrUnavailable
	}
	defer u.session.release(context.Background())
	if current.DirectoryRevision != "" {
		expected, err := mcpserver.ConnectorSource(u.source.InstallationID)
		if err != nil || expected != current.Source {
			return errors.Join(plugin.ErrStale, err)
		}
	}
	change := wire.Change{Name: name.String(), Expected: current.DirectoryRevision}
	var grant string
	if s != nil {
		if operation.Identity(ctx) == "" {
			ctx = operation.Start(ctx, extension.Automatic, c.methods.Change.Name(), "")
		}
		invocation, err := operation.Capture(ctx, portable.Target{Scope: extension.ScopeInstallation, Reference: u.source.InstallationID.String()})
		if err != nil {
			return err
		}
		timeout, _ := s.HandshakeTimeout.Duration()
		change.Replacement = &wire.Entry{Name: name.String(), Description: s.Description, Enabled: s.Enabled, HandshakeSeconds: int64(timeout / time.Second)}
		proposal := app.GrantProposal{Source: u.source.ContributionSource(), Invocation: invocation, Change: change}
		grant, err = c.vault.PrepareGrant(ctx, u.source.InstallationID, *s, proposal)
		if err != nil {
			return err
		}
		change.Replacement.LaunchGrant = grant
	}
	decision, err := connectorCall(ctx, u, c.methods.Change, change)
	if err != nil {
		return err
	}
	if decision != wire.Applied {
		refusal := plugin.ErrStale
		if current.DirectoryRevision == "" {
			refusal = app.ErrServerAlreadyExists
		}
		if grant != "" && decision == wire.Conflict {
			return errors.Join(refusal, c.vault.RejectGrant(context.WithoutCancel(ctx), u.source.InstallationID, name, grant))
		}
		return refusal
	}
	return c.vault.RetireGrants(context.WithoutCancel(ctx), u.source.InstallationID, name, grant)
}

// A directory listing can nominate a token but cannot activate a prepared
// grant. Only the original connector's receipt permits the host to open it.
func (c *ExternalConnector) reconcileGrant(ctx context.Context, u *connectorUse, name mcpserver.ServerName, token string) error {
	proposal, pending, err := c.vault.PendingGrant(ctx, u.source.InstallationID, name, token)
	if err != nil {
		return err
	}
	if !pending {
		return nil
	}
	if proposal.Source != u.source.ContributionSource() || proposal.Change.Replacement == nil {
		return plugin.ErrStale
	}
	proposal.Change.Replacement.LaunchGrant = token
	method, err := c.methods.Change.Bind(u.capability)
	if err != nil {
		return err
	}
	ctx, err = qimumcp.WithInvocation(ctx, proposal.Invocation)
	if err != nil {
		return err
	}
	d, found, err := method.ReadReceipt(ctx, qimumcp.CommandID(proposal.Invocation.ID), proposal.Change)
	if err != nil {
		return err
	}
	if !found {
		return plugin.ErrUnavailable
	}
	switch d {
	case wire.Applied:
		return c.vault.ConfirmPendingGrant(ctx, u.source.InstallationID, name, token)
	case wire.Conflict:
		return errors.Join(plugin.ErrStale, c.vault.RejectGrant(ctx, u.source.InstallationID, name, token))
	default:
		return app.ErrInvalidServerConfiguration
	}
}

func (c *ExternalConnector) ListExposure(ctx context.Context) ([]tool.Ref, error) {
	return c.vault.ListExposure(ctx)
}
func (c *ExternalConnector) SetToolExposure(ctx context.Context, ref tool.Ref, disabled bool) error {
	return c.vault.SetToolExposure(ctx, ref, disabled)
}

func (c *ExternalConnector) Affected(servers []mcpserver.ID) bool {
	selected := c.selected.Load()
	if selected == nil {
		return false
	}
	for _, s := range servers {
		id, ok := s.Origin().Installation()
		if ok && id == *selected {
			return true
		}
	}
	return false
}

func (c *ExternalConnector) recover(ctx context.Context, u *connectorUse) error {
	c.recoveryMu.Lock()
	defer c.recoveryMu.Unlock()
	id := u.source.InstallationID
	if c.recovered[id] == u.epoch {
		return nil
	}
	recovery := operation.Start(ctx, extension.Automatic, c.methods.Recover.Name(), u.epoch)
	decision, err := connectorCall(recovery, u, c.methods.Recover, wire.Recovery{Epoch: u.epoch})
	if err != nil {
		return err
	}
	if decision != wire.Applied {
		return app.ErrInvalidServerConfiguration
	}
	c.recovered[id] = u.epoch
	return nil
}

func (c *ExternalConnector) Invalidate() { epoch := rand.Text(); c.epoch.Store(&epoch) }
