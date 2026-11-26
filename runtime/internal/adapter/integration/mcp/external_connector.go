package mcp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	wire "github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	connectormcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcpconnector/mcp"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/frame"

	framecall "github.com/Tangerg/frame/call"
	"github.com/samber/lo"
)

type directoryVault interface {
	PrepareDirectoryChange(context.Context, mcpapp.PendingDirectoryChange, *mcpserver.Server) (mcpapp.PendingDirectoryChange, error)
	PendingDirectoryChange(context.Context, operation.ReplayClaim) (mcpapp.PendingDirectoryChange, bool, error)
	PendingDirectoryChanges(context.Context) ([]mcpapp.PendingDirectoryChange, error)
	CompleteDirectoryChange(context.Context, mcpapp.PendingDirectoryChange, bool, []byte) error
	OpenGrant(context.Context, identity.InstallationID, mcpserver.ServerName, string) (mcpapp.LaunchGrant, bool, error)
}

type ExternalConnector struct {
	recoveryMu    sync.Mutex
	recovered     map[identity.InstallationID]string
	installed     *Installed
	installations plugins.Store
	selections    plugins.SelectionStore
	vault         directoryVault
	encodeReceipt func(error) ([]byte, error)
	methods       connectormcp.Methods
	epoch         atomic.Pointer[string]
}

func NewExternalConnector(installed *Installed, installations plugins.Store, selections plugins.SelectionStore, vault directoryVault, encodeReceipt func(error) ([]byte, error)) (*ExternalConnector, error) {
	if installed == nil || lo.IsNil(installations) || lo.IsNil(selections) || lo.IsNil(vault) || encodeReceipt == nil {
		return nil, errors.New("external mcp: installed sessions, installations, selections and directory receipts are required")
	}
	m, err := connectormcp.NewMethods()
	if err != nil {
		return nil, err
	}
	connector := &ExternalConnector{recovered: make(map[identity.InstallationID]string), installed: installed, installations: installations, selections: selections, vault: vault, methods: m, encodeReceipt: encodeReceipt}
	connector.Invalidate()
	return connector, nil
}

type connectorUse struct {
	epoch      string
	source     plugins.ContributionBinding
	session    *installedSession
	capability *pluginmcp.CapabilityBinding
}

func (c *ExternalConnector) acquire(ctx context.Context) (*connectorUse, error) {
	b, found, err := plugins.SelectedContribution(ctx, c.installed.host, c.installations, c.selections, c.methods.Contract)
	if err != nil || !found {
		return nil, err
	}
	return c.acquireBinding(ctx, b)
}

func (c *ExternalConnector) acquireBinding(ctx context.Context, b plugins.ContributionBinding) (*connectorUse, error) {
	session, err := c.installed.acquire(ctx, b.ServerID)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*connectorUse, error) {
		return nil, errors.Join(err, session.release(context.WithoutCancel(ctx)))
	}
	activation, err := session.activation()
	if err != nil {
		return fail(err)
	}
	if _, err := plugins.OriginalActivation([]plugin.Activation{activation}, b.InstallationID, b.Digest, b.ServerName); err != nil {
		return fail(err)
	}
	capability, err := pluginmcp.ResolveCapability(ctx, session.session, b.Export, c.methods.Contract)
	if err != nil {
		return fail(err)
	}
	u := &connectorUse{epoch: *c.epoch.Load(), source: b, session: session, capability: capability}
	if err := c.recover(ctx, u); err != nil {
		return fail(err)
	}
	return u, nil
}

func connectorCall[P, R any](ctx context.Context, u *connectorUse, m *pluginmcp.Method[P, R], input P) (R, error) {
	var zero R
	method, err := m.Bind(u.capability)
	if err != nil {
		return zero, err
	}
	if operation.Identity(ctx) == "" {
		ctx = operation.Start(ctx, extension.Automatic, m.Name(), "")
	}
	invocation, err := operation.Capture(ctx, framecall.Target{Scope: extension.ScopeInstallation, Reference: u.source.InstallationID.String()})
	if err != nil {
		return zero, err
	}
	ctx, err = pluginmcp.WithInvocation(ctx, invocation)
	if err != nil {
		return zero, err
	}
	if m.Descriptor().Kind == frame.Query {
		return method.Call(ctx, input)
	}
	return method.CallCommand(ctx, pluginmcp.CommandID(operation.Identity(ctx)), input)
}

func (c *ExternalConnector) entries(ctx context.Context, u *connectorUse, name string) (wire.Directory, error) {
	directory, err := connectorCall(ctx, u, c.methods.List, wire.Selector{Name: name})
	if err != nil {
		return wire.Directory{}, err
	}
	seen := make(map[mcpserver.ServerName]struct{}, len(directory.Entries))
	for _, entry := range directory.Entries {
		identity, err := mcpserver.ParseServerName(entry.Name)
		if err != nil {
			return wire.Directory{}, errors.Join(mcpapp.ErrInvalidServerConfiguration, err)
		}
		if entry.Revision == "" || entry.LaunchGrant == "" || name != "" && entry.Name != name {
			return wire.Directory{}, mcpapp.ErrInvalidServerConfiguration
		}
		if _, duplicate := seen[identity]; duplicate {
			return wire.Directory{}, mcpapp.ErrInvalidServerConfiguration
		}
		seen[identity] = struct{}{}
	}
	return directory, nil
}

func (c *ExternalConnector) materialize(ctx context.Context, u *connectorUse, e wire.Entry) (mcpserver.Server, error) {
	name, err := mcpserver.ParseServerName(e.Name)
	if err != nil {
		return mcpserver.Server{}, err
	}
	grant, found, err := c.vault.OpenGrant(ctx, u.source.InstallationID, name, e.LaunchGrant)
	if err != nil {
		return mcpserver.Server{}, err
	}
	if !found {
		return mcpserver.Server{}, fmt.Errorf("%w: external transport grant is unavailable", mcpapp.ErrInvalidServerConfiguration)
	}
	if grant.Proposal != nil {
		if err := c.verifyGrant(ctx, u, *grant.Proposal, e.LaunchGrant); err != nil {
			return mcpserver.Server{}, err
		}
	}
	source, err := mcpserver.ConnectorSource(u.source.InstallationID)
	if err != nil {
		return mcpserver.Server{}, err
	}
	s := mcpserver.Server{DirectoryRevision: e.Revision, Name: name, Source: source, Connection: grant.Connection, Enabled: e.Enabled, Description: e.Description}
	if e.HandshakeSeconds != 0 {
		if e.HandshakeSeconds < 0 || e.HandshakeSeconds > int64(math.MaxInt64/time.Second) {
			return s, mcpapp.ErrInvalidServerConfiguration
		}
		s.HandshakeTimeout, err = mcpserver.NewHandshakeTimeout(time.Duration(e.HandshakeSeconds) * time.Second)
		if err != nil {
			return s, err
		}
	}
	return s, s.Validate()
}

// A failed release is reported with the operation's own result. selected
// is false when no connector contribution is selected.
func (c *ExternalConnector) use(ctx context.Context, operate func(*connectorUse) error) (selected bool, err error) {
	u, err := c.acquire(ctx)
	if err != nil || u == nil {
		return false, err
	}
	defer func() { err = errors.Join(err, u.session.release(context.WithoutCancel(ctx))) }()
	return true, operate(u)
}

func (c *ExternalConnector) List(ctx context.Context) ([]mcpserver.Server, error) {
	var servers []mcpserver.Server
	if _, err := c.use(ctx, func(u *connectorUse) error {
		d, err := c.entries(ctx, u, "")
		if err != nil {
			return err
		}
		servers = make([]mcpserver.Server, 0, len(d.Entries))
		for _, e := range d.Entries {
			s, err := c.materialize(ctx, u, e)
			if err != nil {
				return err
			}
			servers = append(servers, s)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return servers, nil
}

func (c *ExternalConnector) Get(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error) {
	var server mcpserver.Server
	found := false
	_, err := c.use(ctx, func(u *connectorUse) error {
		e, ok, err := c.find(ctx, u, name)
		if err != nil || !ok {
			return err
		}
		server, err = c.materialize(ctx, u, e)
		found = err == nil
		return err
	})
	return server, found && err == nil, err
}

func (c *ExternalConnector) find(ctx context.Context, u *connectorUse, name mcpserver.ServerName) (wire.Entry, bool, error) {
	d, err := c.entries(ctx, u, name.String())
	if err != nil || len(d.Entries) == 0 {
		return wire.Entry{}, false, err
	}
	return d.Entries[0], true, nil
}

func (c *ExternalConnector) Affected(ctx context.Context, servers []mcpserver.ID) (bool, error) {
	if len(servers) == 0 {
		return false, nil
	}
	selection, configured, err := c.selections.Get(ctx, wire.Reference)
	if err != nil || !configured {
		return false, err
	}
	selected := selection.Snapshot().Installation
	for _, s := range servers {
		id, ok := s.Origin().Installation()
		if ok && id == selected {
			return true, nil
		}
	}
	return false, nil
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
		return mcpapp.ErrInvalidServerConfiguration
	}
	c.recovered[id] = u.epoch
	return nil
}

func (c *ExternalConnector) Invalidate() { epoch := rand.Text(); c.epoch.Store(&epoch) }
