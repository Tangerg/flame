// Package runtimebinding adapts the public Runtime bindings and owns
// the immutable capability profile negotiated for CLI consumers. Validated
// management, catalog, and workspace results transfer ownership to the caller.
// Retained profiles and live event projections acquire their own mutable values.
// Calls borrow the connection's immutable request metadata; Runtime owns the
// snapshot retained by an operation or stream.
package runtimebinding

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	runtimeclient "github.com/Tangerg/flame/packages/runtime-client/go"

	"github.com/Tangerg/flame/apps/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

const clientName = "flame-cli"

func supportedInterruptTypes() []protocol.InterruptType {
	return []protocol.InterruptType{
		protocol.InterruptApproval,
		protocol.InterruptQuestion,
	}
}

func recognizedRunEventTypes() []protocol.StreamEventType {
	return []protocol.StreamEventType{
		protocol.StreamSegmentStarted,
		protocol.StreamSegmentProgress,
		protocol.StreamSegmentFinished,
		protocol.StreamItemStarted,
		protocol.StreamItemDelta,
		protocol.StreamItemCompleted,
		protocol.StreamPlanUpdated,
	}
}

func requiredRunEventTypes() []protocol.StreamEventType {
	return []protocol.StreamEventType{
		protocol.StreamSegmentStarted,
		protocol.StreamSegmentFinished,
		protocol.StreamItemStarted,
		protocol.StreamItemDelta,
		protocol.StreamItemCompleted,
		protocol.StreamPlanUpdated,
	}
}

// Config contains local Runtime paths, remote authorization, and client identity.
// Paths retain the semantics documented by flameruntime.Config.
type Config struct {
	// ProductRoot is FLAME_HOME. Runtime's durability lives at a segment beneath
	// it that localruntime owns and publishes for every local surface, so this
	// boundary resolves it rather than accepting a composed path.
	ProductRoot          string
	DefaultWorkspacePath string
	UserHomePath         string
	ConfigDirectories    []string
	ClientVersion        string
	RemoteToken          string
}

type runtimeLifecycle interface {
	Close() error
}

type discoveryBinding interface {
	Discover(context.Context, protocol.CallOptions) (*protocol.DiscoverResponse, error)
}

// runtimeBinding is the composition boundary shared by the embedded Runtime
// and its public HTTP client. Consumers continue to receive their narrow ports.
type runtimeBinding interface {
	runtimeLifecycle
	discoveryBinding
	modelCatalogBinding
	approvalBinding
	sessionCatalogBinding
	snapshotBinding
	runCatalogBinding
	runBinding
	sessionBinding
	workspaceBinding
	changeBinding
	usageBinding
	modelConfigBinding
	goalBinding
	skillBinding
	mcpBinding
	pluginBinding
	diagnosticToolBinding
	authoringContextBinding
	hookBinding
	feedbackBinding
}

// Connection owns one negotiated Runtime binding and translates its protocol
// DTOs into CLI-owned consumer interfaces. It owns no product state machine.
type Connection struct {
	lifecycle        runtimeLifecycle
	discovery        discoveryBinding
	modelCatalog     modelCatalogBinding
	approvals        approvalBinding
	sessionCatalog   sessionCatalogBinding
	snapshot         snapshotBinding
	runCatalog       runCatalogBinding
	runs             runBinding
	sessions         sessionBinding
	workspaces       workspaceBinding
	changes          changeBinding
	usage            usageBinding
	modelConfig      modelConfigBinding
	goals            goalBinding
	skills           skillBinding
	plugins          pluginBinding
	mcp              mcpBinding
	diagnosticTools  diagnosticToolBinding
	authoringContext authoringContextBinding
	hooks            hookBinding
	feedback         feedbackBinding
	meta             protocol.RequestMeta
	loadAttachment   attachmentLoader
	profile          Profile
}

var _ changefeed.Source = (*Connection)(nil)

func openEmbeddedBinding(ctx context.Context, cfg Config) (*flameruntime.Runtime, error) {
	dataDirectory, err := localruntime.DataDirectoryUnder(cfg.ProductRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime data directory: %w", err)
	}
	configDirectories := cfg.ConfigDirectories
	if len(configDirectories) == 0 {
		configDirectories = []string{dataDirectory.Path()}
	}
	return flameruntime.Open(ctx, flameruntime.Config{
		DataDirectory:        dataDirectory.Path(),
		DefaultWorkspacePath: cfg.DefaultWorkspacePath,
		UserHomePath:         cfg.UserHomePath,
		ConfigDirectories:    configDirectories,
	})
}

func openConnection(ctx context.Context, cfg Config, target settings.Runtime) (*Connection, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	var binding runtimeBinding
	var err error
	if target.Mode == settings.RuntimeEmbedded {
		binding, err = openEmbeddedBinding(ctx, cfg)
	} else {
		binding, err = runtimeclient.Connect(ctx, runtimeclient.RemoteConfig{Endpoint: target.Endpoint, Token: cfg.RemoteToken})
	}
	if err != nil {
		return nil, classifyError(err)
	}

	connection := &Connection{
		lifecycle:        binding,
		discovery:        binding,
		modelCatalog:     binding,
		approvals:        binding,
		sessionCatalog:   binding,
		snapshot:         binding,
		runCatalog:       binding,
		runs:             binding,
		sessions:         binding,
		workspaces:       binding,
		changes:          binding,
		usage:            binding,
		modelConfig:      binding,
		goals:            binding,
		skills:           binding,
		plugins:          binding,
		mcp:              binding,
		diagnosticTools:  binding,
		authoringContext: binding,
		hooks:            binding,
		feedback:         binding,
		meta:             requestMeta(cfg.ClientVersion),
		loadAttachment:   loadAttachmentFile,
	}
	discovery, err := connection.discovery.Discover(ctx, connection.callOptions())
	if err == nil {
		connection.profile, err = NewProfile(discovery, connection.meta.ClientCapabilities)
	}
	if err != nil {
		return connection, classifyError(err)
	}
	return connection, nil
}

func requestMeta(version string) protocol.RequestMeta {
	if version == "" {
		version = "dev"
	}
	return protocol.RequestMeta{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientInfo:      &protocol.ClientInfo{Name: clientName, Version: version},
		ClientCapabilities: &protocol.ClientCapabilities{
			Features: map[string]protocol.FeaturePreference{
				protocol.FeatureSubagents: {Enabled: true},
			},
			InterruptTypes: supportedInterruptTypes(),
		},
	}
}

func (r *Connection) callOptions() protocol.CallOptions {
	return protocol.CallOptions{RequestMeta: r.meta}
}

func (r *Connection) commandOptions() protocol.CommandOptions {
	key := string(mutation.NewCommandID())
	return protocol.CommandOptions{
		RequestMeta: r.meta, IdempotencyKey: key,
		IdempotencyNamespace: r.profile.IdempotencyLimits().Namespace,
	}
}

func (r *Connection) commandOptionsFor(commandID replay.CommandID) (protocol.CommandOptions, error) {
	if commandID == "" {
		return r.commandOptions(), nil
	}
	if err := commandID.Validate(); err != nil {
		return protocol.CommandOptions{}, err
	}
	return protocol.CommandOptions{
		RequestMeta: r.meta, IdempotencyKey: string(commandID),
		IdempotencyNamespace: r.profile.IdempotencyLimits().Namespace,
	}, nil
}

func (r *Connection) runCommandOptions() protocol.RunCommandOptions {
	key := string(mutation.NewCommandID())
	return protocol.RunCommandOptions{
		RequestMeta: r.meta, IdempotencyKey: key,
		IdempotencyNamespace: r.profile.IdempotencyLimits().Namespace,
	}
}

func (r *Connection) runCommandOptionsFor(commandID replay.CommandID) (protocol.RunCommandOptions, error) {
	if commandID == "" {
		return r.runCommandOptions(), nil
	}
	if err := commandID.Validate(); err != nil {
		return protocol.RunCommandOptions{}, err
	}
	return protocol.RunCommandOptions{
		RequestMeta: r.meta, IdempotencyKey: string(commandID),
		IdempotencyNamespace: r.profile.IdempotencyLimits().Namespace,
	}, nil
}

func (r *Connection) subscriptionOptions(afterEventID string) (protocol.RunSubscriptionOptions, error) {
	if afterEventID != "" {
		if len(afterEventID) > protocol.MaximumRunEventIDCharacters {
			return protocol.RunSubscriptionOptions{}, fmt.Errorf(
				"run replay cursor exceeds the %d-character transport limit",
				protocol.MaximumRunEventIDCharacters,
			)
		}
		if !strings.HasPrefix(afterEventID, protocol.IDPrefixEvent) {
			return protocol.RunSubscriptionOptions{}, errors.New("run replay cursor has invalid event-id framing")
		}
	}
	return protocol.RunSubscriptionOptions{
		RequestMeta:  r.meta,
		AfterEventID: afterEventID,
	}, nil
}

func (r *Connection) changeSubscriptionOptions() protocol.SubscriptionOptions {
	return protocol.SubscriptionOptions{RequestMeta: r.meta}
}

func validateDiscovery(discovery *protocol.DiscoverResponse) error {
	if discovery == nil {
		return fmt.Errorf("%w: discovery response is nil", conversation.ErrIncompatibleRuntime)
	}
	if discovery.ProtocolVersion != protocol.ProtocolVersion {
		return fmt.Errorf(
			"%w: runtime serves %s, CLI requires %s",
			conversation.ErrIncompatibleRuntime,
			discovery.ProtocolVersion,
			protocol.ProtocolVersion,
		)
	}
	if discovery.Capabilities.Limits.RunReplay.Scope != protocol.ReplayScopeRuntimeInstanceRootSegment {
		return fmt.Errorf("%w: unsupported run replay scope %q", conversation.ErrIncompatibleRuntime, discovery.Capabilities.Limits.RunReplay.Scope)
	}
	for _, method := range []string{"runs.start", "runs.resume", "runs.subscribe"} {
		if !slices.Contains(discovery.Capabilities.StreamingMethods, method) {
			return fmt.Errorf("%w: runtime does not stream %s", conversation.ErrIncompatibleRuntime, method)
		}
	}
	for _, eventType := range discovery.Capabilities.RunEvents {
		if !slices.Contains(recognizedRunEventTypes(), eventType) {
			return fmt.Errorf("%w: runtime advertises unsupported run event %q", conversation.ErrIncompatibleRuntime, eventType)
		}
	}
	for _, eventType := range requiredRunEventTypes() {
		if !slices.Contains(discovery.Capabilities.RunEvents, eventType) {
			return fmt.Errorf("%w: runtime does not advertise %s", conversation.ErrIncompatibleRuntime, eventType)
		}
	}
	for _, topic := range discovery.Capabilities.RuntimeTopics {
		if !slices.Contains(changefeed.Topics(), topic) {
			return fmt.Errorf("%w: runtime advertises unsupported change topic %q", conversation.ErrIncompatibleRuntime, topic)
		}
	}
	return nil
}

// Close releases the selected binding. The embedded binding tears down its own
// Runtime; the remote binding only detaches this client's calls and streams.
func (r *Connection) Close() error {
	return classifyError(r.lifecycle.Close())
}

// Owner lazily opens exactly one Runtime connection for a process and owns its
// shutdown. A failed open may be retried after successful cleanup; incomplete
// cleanup moves the owner into shutdown so it cannot create a second Runtime.
type Owner struct {
	mu         sync.Mutex
	config     Config
	connection *Connection
	target     settings.Runtime
	closing    bool
}

func NewOwner(config Config) *Owner {
	config.ConfigDirectories = slices.Clone(config.ConfigDirectories)
	return &Owner{config: config}
}

func (o *Owner) Connection(ctx context.Context, target settings.Runtime) (*Connection, error) {
	if o == nil {
		return nil, errors.New("runtime connection owner is nil")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closing {
		return nil, conversation.ErrDisconnected
	}
	if o.connection != nil {
		if target != o.target {
			return nil, errors.New("runtime target cannot change during a CLI process")
		}
		return o.connection, nil
	}
	opened, err := openConnection(ctx, o.config, target)
	if err != nil {
		if opened != nil {
			err = o.rejectOpen(opened, err)
		}
		return nil, err
	}
	o.connection = opened
	o.target = target
	return opened, nil
}

func (o *Owner) rejectOpen(opened *Connection, openErr error) error {
	closeErr := opened.Close()
	if closeErr == nil {
		return openErr
	}
	o.connection = opened
	o.closing = true
	return errors.Join(openErr, fmt.Errorf("close runtime after failed negotiation: %w", closeErr))
}

// Profile returns the immutable discovery projection for this connection.
func (r *Connection) Profile() Profile { return r.profile }

// AgentMemory and the accessors below hand out a usable binding for
// their surface. Whether the connected Runtime offers that surface is the
// composition root's question, asked once against the Profile: an accessor that
// answered it by returning a nil pointer would arrive at a consumer's interface
// field as a non-nil interface, and defeat the very check that asks.
func (r *Connection) AgentMemory() *AgentMemory {
	return &AgentMemory{runtime: r}
}

func (r *Connection) DiagnosticTools() *DiagnosticTools {
	return &DiagnosticTools{runtime: r}
}

func (r *Connection) AuthoringContext() *AuthoringContext {
	return &AuthoringContext{runtime: r}
}

func (r *Connection) Hooks() *Hooks {
	return &Hooks{runtime: r}
}

func (r *Connection) Feedback() *Feedback {
	return &Feedback{runtime: r}
}

func (r *Connection) supportsFeature(name string) bool {
	return r.profile.Supports(name)
}

func (r *Connection) requireFeature(name string) error {
	if r.supportsFeature(name) {
		return nil
	}
	return fmt.Errorf("%w: runtime capability %q was not negotiated", conversation.ErrIncompatibleRuntime, name)
}

func (o *Owner) Close() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closing = true
	connection := o.connection
	if connection == nil {
		return nil
	}
	err := connection.Close()
	if err == nil {
		o.connection = nil
	}
	return err
}
