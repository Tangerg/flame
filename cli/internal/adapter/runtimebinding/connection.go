// Package runtimebinding adapts the public in-process Runtime binding and owns
// the immutable capability profile negotiated for CLI consumers. Validated
// management, catalog, and workspace results transfer ownership to the caller.
// Retained profiles and live event projections acquire their own mutable values.
package runtimebinding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
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

// Config contains the process-owned paths and build identity needed to open one
// in-process Runtime. Paths retain the semantics documented by flameruntime.Config.
type Config struct {
	DataDirectory        string
	DefaultWorkspacePath string
	UserHomePath         string
	ConfigDirectories    []string
	ClientVersion        string
}

type runtimeLifecycle interface {
	Close() error
}

type discoveryBinding interface {
	Discover(context.Context, flameruntime.CallOptions) (*protocol.DiscoverResponse, error)
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
	mcp              mcpBinding
	schedules        scheduleBinding
	agentMemory      agentMemoryBinding
	knowledge        knowledgeBinding
	diagnosticTools  diagnosticToolBinding
	authoringContext authoringContextBinding
	hooks            hookBinding
	feedback         feedbackBinding
	meta             protocol.RequestMeta
	loadAttachment   attachmentLoader
	profile          Profile
}

var _ changefeed.Source = (*Connection)(nil)

func openConnection(ctx context.Context, cfg Config) (*Connection, error) {
	binding, err := flameruntime.Open(ctx, flameruntime.Config{
		DataDirectory:        cfg.DataDirectory,
		DefaultWorkspacePath: cfg.DefaultWorkspacePath,
		UserHomePath:         cfg.UserHomePath,
		ConfigDirectories:    slices.Clone(cfg.ConfigDirectories),
	})
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
		mcp:              binding,
		schedules:        binding,
		agentMemory:      binding,
		knowledge:        binding,
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

func (r *Connection) callOptions() flameruntime.CallOptions {
	return flameruntime.CallOptions{RequestMeta: cloneRequestMeta(r.meta)}
}

func (r *Connection) commandOptions() (flameruntime.CommandOptions, error) {
	key, err := newIdempotencyKey()
	if err != nil {
		return flameruntime.CommandOptions{}, err
	}
	return flameruntime.CommandOptions{
		RequestMeta: cloneRequestMeta(r.meta), IdempotencyKey: key,
		IdempotencyNamespace: r.profile.discovery.Capabilities.Limits.Idempotency.Namespace,
	}, nil
}

func (r *Connection) commandOptionsFor(commandID agent.CommandID) (flameruntime.CommandOptions, error) {
	if commandID == "" {
		return r.commandOptions()
	}
	if err := commandID.Validate(); err != nil {
		return flameruntime.CommandOptions{}, err
	}
	return flameruntime.CommandOptions{
		RequestMeta: cloneRequestMeta(r.meta), IdempotencyKey: string(commandID),
		IdempotencyNamespace: r.profile.discovery.Capabilities.Limits.Idempotency.Namespace,
	}, nil
}

func (r *Connection) runCommandOptions() (flameruntime.RunCommandOptions, error) {
	key, err := newIdempotencyKey()
	if err != nil {
		return flameruntime.RunCommandOptions{}, err
	}
	return flameruntime.RunCommandOptions{
		RequestMeta: cloneRequestMeta(r.meta), IdempotencyKey: key,
		IdempotencyNamespace: r.profile.discovery.Capabilities.Limits.Idempotency.Namespace,
	}, nil
}

func (r *Connection) runCommandOptionsFor(commandID agent.CommandID) (flameruntime.RunCommandOptions, error) {
	if commandID == "" {
		return r.runCommandOptions()
	}
	if err := commandID.Validate(); err != nil {
		return flameruntime.RunCommandOptions{}, err
	}
	return flameruntime.RunCommandOptions{
		RequestMeta: cloneRequestMeta(r.meta), IdempotencyKey: string(commandID),
		IdempotencyNamespace: r.profile.discovery.Capabilities.Limits.Idempotency.Namespace,
	}, nil
}

func (r *Connection) subscriptionOptions(afterEventID string) (flameruntime.RunSubscriptionOptions, error) {
	if afterEventID != "" {
		if len(afterEventID) > protocol.MaximumRunEventIDCharacters {
			return flameruntime.RunSubscriptionOptions{}, fmt.Errorf(
				"run replay cursor exceeds the %d-character transport limit",
				protocol.MaximumRunEventIDCharacters,
			)
		}
		if !strings.HasPrefix(afterEventID, protocol.IDPrefixEvent) {
			return flameruntime.RunSubscriptionOptions{}, errors.New("run replay cursor has invalid event-id framing")
		}
	}
	return flameruntime.RunSubscriptionOptions{
		RequestMeta:  cloneRequestMeta(r.meta),
		AfterEventID: afterEventID,
	}, nil
}

func (r *Connection) changeSubscriptionOptions() flameruntime.SubscriptionOptions {
	return flameruntime.SubscriptionOptions{RequestMeta: cloneRequestMeta(r.meta)}
}

func cloneRequestMeta(meta protocol.RequestMeta) protocol.RequestMeta {
	cloned := meta
	if meta.ClientInfo != nil {
		cloned.ClientInfo = new(*meta.ClientInfo)
	}
	cloned.ClientCapabilities = cloneClientCapabilities(meta.ClientCapabilities)
	return cloned
}

func newIdempotencyKey() (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate runtime idempotency key: %w", err)
	}
	return "cli_" + hex.EncodeToString(entropy[:]), nil
}

func validateDiscovery(discovery *protocol.DiscoverResponse) error {
	if discovery == nil {
		return fmt.Errorf("%w: discovery response is nil", agent.ErrIncompatibleRuntime)
	}
	if err := protocol.ValidateWireTree(*discovery); err != nil {
		return runtimeContractViolation("discovery response violates the wire contract: %v", err)
	}
	if discovery.ProtocolVersion != protocol.ProtocolVersion {
		return fmt.Errorf(
			"%w: runtime serves %s, CLI requires %s",
			agent.ErrIncompatibleRuntime,
			discovery.ProtocolVersion,
			protocol.ProtocolVersion,
		)
	}
	if discovery.Capabilities.Limits.RunReplay.Scope != protocol.ReplayScopeRuntimeInstanceRootSegment {
		return fmt.Errorf("%w: unsupported run replay scope %q", agent.ErrIncompatibleRuntime, discovery.Capabilities.Limits.RunReplay.Scope)
	}
	for _, method := range []string{"runs.start", "runs.resume", "runs.subscribe"} {
		if !slices.Contains(discovery.Capabilities.StreamingMethods, method) {
			return fmt.Errorf("%w: runtime does not stream %s", agent.ErrIncompatibleRuntime, method)
		}
	}
	for _, eventType := range discovery.Capabilities.RunEvents {
		if !slices.Contains(recognizedRunEventTypes(), eventType) {
			return fmt.Errorf("%w: runtime advertises unsupported run event %q", agent.ErrIncompatibleRuntime, eventType)
		}
	}
	for _, eventType := range requiredRunEventTypes() {
		if !slices.Contains(discovery.Capabilities.RunEvents, eventType) {
			return fmt.Errorf("%w: runtime does not advertise %s", agent.ErrIncompatibleRuntime, eventType)
		}
	}
	for _, topic := range discovery.Capabilities.RuntimeTopics {
		if !slices.Contains(changefeed.Topics(), topic) {
			return fmt.Errorf("%w: runtime advertises unsupported change topic %q", agent.ErrIncompatibleRuntime, topic)
		}
	}
	return nil
}

// Close completes the in-process Runtime teardown. Call it again when it returns
// an error; flameruntime.Runtime.Close resumes incomplete teardown.
func (r *Connection) Close() error {
	if r == nil || r.lifecycle == nil {
		return nil
	}
	return classifyError(r.lifecycle.Close())
}

// Owner lazily opens exactly one Runtime connection for a process and owns its
// shutdown. A failed open may be retried after successful cleanup; incomplete
// cleanup moves the owner into shutdown so it cannot create a second Runtime.
type Owner struct {
	mu         sync.Mutex
	config     Config
	connection *Connection
	closing    bool
}

func NewOwner(config Config) *Owner {
	config.ConfigDirectories = slices.Clone(config.ConfigDirectories)
	return &Owner{config: config}
}

func (o *Owner) Connection(ctx context.Context) (*Connection, error) {
	if o == nil {
		return nil, errors.New("runtime connection owner is nil")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closing {
		return nil, agent.ErrDisconnected
	}
	if o.connection != nil {
		return o.connection, nil
	}
	opened, err := openConnection(ctx, o.config)
	if err != nil {
		if opened != nil {
			err = o.rejectOpen(opened, err)
		}
		return nil, err
	}
	o.connection = opened
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

func (r *Connection) AgentMemory() *AgentMemory {
	if !r.supportsFeature(protocol.FeatureAgentMemory) {
		return nil
	}
	return &AgentMemory{runtime: r}
}

func (r *Connection) Knowledge() *Knowledge {
	if !r.supportsFeature(protocol.FeatureKnowledge) {
		return nil
	}
	return &Knowledge{runtime: r}
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
	return fmt.Errorf("%w: runtime capability %q was not negotiated", agent.ErrIncompatibleRuntime, name)
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
