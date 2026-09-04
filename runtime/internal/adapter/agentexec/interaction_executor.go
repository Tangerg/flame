package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

const (
	interactionDefinitionName        = "flame.runtime.interaction"
	interactionDefinitionDescription = "Run one model-directed Flame interaction over a frozen working context."
	defaultInteractionModelCalls     = 64
	interactionEventBuffer           = 64
	interactionReleaseReason         = "runtime released execution resources"
	defaultUnknownEffectPollInterval = time.Second
	defaultInteractionStatePoll      = 250 * time.Millisecond
	interactionDoomLoopThreshold     = 3
)

// InteractionChatResolver resolves one exact provider construction selected by
// a Run. Resolution happens during staging and must not invoke the model.
type InteractionChatResolver interface {
	ResolveChat(ctx context.Context, selection modelref.Selection) (modeladapter.ResolvedChat, error)
}

// RestoreScopeValidator verifies the host facts a durable executor checkpoint
// cannot prove for itself. It must not mutate or recreate the workspace.
type RestoreScopeValidator interface {
	ValidateRestoreScope(ctx context.Context, scope runs.ExecutionScope) error
}

// InteractionExecutorConfig freezes the host-owned inputs shared by
// Interaction root executions. Identity strings must change whenever the
// executable Interaction adapter or behavior-affecting dispatcher configuration
// changes, so Agent Framework Deployment references remain honest.
type InteractionExecutorConfig struct {
	// Lifetime is the process-owned root for every Interaction staged by this
	// executor. Request contexts may bound staging and commands, but accepted
	// execution must outlive the request that created it.
	Lifetime                  context.Context
	BuildID                   string
	ChatResolver              InteractionChatResolver
	RestoreScopeValidator     RestoreScopeValidator
	ImplementationIdentity    string
	ConfigurationIdentity     string
	DefaultMaxModelCalls      *uint32
	StreamModelResponses      bool
	DeltaBufferCapacity       *int
	MaxConcurrentToolCalls    *int
	ToolResolver              InteractionToolResolver
	ToolInterpreter           InteractionToolInterpreter
	ToolPresenter             InteractionToolPresenter
	ToolAuthorizer            InteractionToolAuthorizer
	ToolHooks                 InteractionToolHooks
	MCPToolAutoApproved       func(server, tool string) bool
	Maintenance               RunMaintenance
	ModelContextCompactor     ModelContextCompactor
	ModelContextState         InteractionModelContextState
	LifecycleHooks            InteractionLifecycleHooks
	ToolResultStore           toolResultOffloader
	ToolResultOffload         ToolResultOffloadPolicyValues
	Pricing                   accounting.Pricing
	UnknownEffectPollInterval *time.Duration
	StatePollInterval         *time.Duration
	Delegation                InteractionDelegationPolicyValues
}

// InteractionExecutor is the Agent Framework root execution adapter. Each staged
// root owns an independent Engine and exactly one Interaction Process; the
// Application owns durable Run state and consumes only [runs.ExecutorEvent].
type InteractionExecutor struct {
	lifetime               context.Context
	config                 InteractionExecutorConfig
	policy                 interactionExecutionPolicy
	buildID                runtimeidentity.BuildID
	implementationIdentity deploymentIdentity
	configurationIdentity  deploymentIdentity

	sessions interactionSessions
}

// NewInteractionExecutor validates immutable host configuration. It creates no
// Engine, goroutine, model call, or tool call; those resources are per staged
// root execution.
func NewInteractionExecutor(config InteractionExecutorConfig) (*InteractionExecutor, error) {
	if config.Lifetime == nil {
		return nil, errors.New("agentexec: Interaction lifetime is required")
	}
	if isNilInteractionCapability(config.ChatResolver) {
		return nil, errors.New("agentexec: Interaction requires a chat resolver")
	}
	if isNilInteractionCapability(config.ModelContextCompactor) !=
		isNilInteractionCapability(config.ModelContextState) {
		return nil, errors.New("agentexec: model-context compactor and state source must be configured together")
	}
	for _, capability := range []struct {
		name  string
		value any
	}{
		{name: "chat resolver", value: config.ChatResolver},
		{name: "restore-scope validator", value: config.RestoreScopeValidator},
		{name: "Tool resolver", value: config.ToolResolver},
		{name: "Tool interpreter", value: config.ToolInterpreter},
		{name: "Tool presenter", value: config.ToolPresenter},
		{name: "Tool authorizer", value: config.ToolAuthorizer},
		{name: "Tool hooks", value: config.ToolHooks},
		{name: "Run maintenance", value: config.Maintenance},
		{name: "model-context compactor", value: config.ModelContextCompactor},
		{name: "model-context state", value: config.ModelContextState},
		{name: "lifecycle hooks", value: config.LifecycleHooks},
		{name: "Tool-result store", value: config.ToolResultStore},
	} {
		if capability.value != nil && isNilInteractionCapability(capability.value) {
			return nil, fmt.Errorf("agentexec: Interaction %s is typed nil", capability.name)
		}
	}
	implementationIdentity, err := parseDeploymentIdentity("deployment implementation identity", config.ImplementationIdentity)
	if err != nil {
		return nil, fmt.Errorf("agentexec: Interaction: %w", err)
	}
	buildID, err := runtimeidentity.ParseBuild(config.BuildID)
	if err != nil {
		return nil, fmt.Errorf("agentexec: Interaction %w", err)
	}
	configurationIdentity, err := parseDeploymentIdentity("deployment configuration identity", config.ConfigurationIdentity)
	if err != nil {
		return nil, fmt.Errorf("agentexec: Interaction: %w", err)
	}
	policy, err := newInteractionExecutionPolicy(config)
	if err != nil {
		return nil, err
	}
	lifetime := config.Lifetime
	config.Lifetime = nil
	config.BuildID = ""
	config.ImplementationIdentity = ""
	config.ConfigurationIdentity = ""
	return &InteractionExecutor{
		lifetime: lifetime, config: config, policy: policy,
		buildID:                buildID,
		implementationIdentity: implementationIdentity,
		configurationIdentity:  configurationIdentity,
		sessions:               newInteractionSessions(),
	}, nil
}

func (i *InteractionExecutor) acceptsBuild(raw string) bool {
	identity, err := runtimeidentity.ParseBuild(raw)
	return err == nil && identity == i.buildID
}

// ValidateRootStart rejects inputs the Interaction cannot represent.
func (i *InteractionExecutor) ValidateRootStart(start runs.RootExecutionStart) error {
	if err := start.Validate(); err != nil {
		return err
	}
	if err := validateModelOutputReservation(start.ModelSelection, start.Options); err != nil {
		return err
	}
	if len(start.WorkingContext) == 0 {
		return errors.New("agentexec: Interaction requires a complete working context")
	}
	_, err := i.maxModelCalls(start)
	return err
}

func validateModelOutputReservation(
	selection modelref.Selection,
	options *corechat.Options,
) error {
	if options == nil || options.MaxOutputTokens == nil {
		return nil
	}
	limits, found, err := modeladapter.LookupTokenLimits(selection)
	if err != nil {
		return fmt.Errorf("%w: %w", runs.ErrInvalidRunOptions, err)
	}
	if !found {
		return nil
	}
	reservation, err := modelref.NewOutputReservation(*options.MaxOutputTokens)
	if err != nil {
		return fmt.Errorf("%w: %w", runs.ErrInvalidRunOptions, err)
	}
	if _, _, err := limits.InputCeiling(reservation); err != nil {
		return fmt.Errorf("%w: %w", runs.ErrInvalidRunOptions, err)
	}
	return nil
}

// StageRoot assembles one exact Interaction Deployment and independent Engine
// without starting a Process or crossing the model/tool side-effect boundary.
func (i *InteractionExecutor) StageRoot(
	ctx context.Context,
	start runs.RootExecutionStart,
) (runs.ExecutorRef, error) {
	if i == nil {
		return runs.ExecutorRef{}, errors.New("agentexec: Interaction executor is nil")
	}
	if _, err := resourceid.ParseSession(start.SessionID); err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("agentexec: Interaction: %w", err)
	}
	if err := i.ValidateRootStart(start); err != nil {
		return runs.ExecutorRef{}, err
	}
	ref := runs.ExecutorRef{SessionID: start.SessionID, ExecutorID: "exec_" + uuid.NewString()}
	session, err := i.assembleInteraction(ctx, ref, start)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	input, err := agent.EncodeInput(interaction.Input{
		Messages: cloneChatMessages(start.WorkingContext), Options: executionOptions(start.ModelSelection, start.Options),
	})
	if err != nil {
		_ = session.engine.Close()
		return runs.ExecutorRef{}, fmt.Errorf("agentexec: encode Interaction input: %w", err)
	}
	session.input = input
	if err := i.sessions.register(session); err != nil {
		_ = session.engine.Close()
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

func (i *InteractionExecutor) assembleInteraction(
	ctx context.Context,
	ref runs.ExecutorRef,
	start runs.RootExecutionStart,
) (*interactionSession, error) {
	start.InterruptKinds = slices.Clone(start.InterruptKinds)
	resolved, err := i.resolveChat(ctx, start.ModelSelection)
	if err != nil {
		return nil, err
	}
	client := resolved.Client()
	counter, _ := resolved.InputTokenCounter()
	maxModelCalls, err := i.maxModelCalls(start)
	if err != nil {
		return nil, err
	}
	allowance, err := newInteractionAllowance(start.Limits, start.ModelSelection, i.config.Pricing)
	if err != nil {
		return nil, err
	}
	session := newInteractionSession(i.lifetime, ref, start, i.config, i.buildID, i.policy)
	session.allowance = allowance
	observedClient, err := newObservedInteractionClient(client, session)
	if err != nil {
		return nil, fmt.Errorf("agentexec: observe Interaction client: %w", err)
	}
	deployments, err := i.buildInteractionDeployments(
		runExecutionContext(ctx, rootExecutionScope(start), start), session, start, observedClient, counter, maxModelCalls,
	)
	if err != nil {
		return nil, err
	}
	if installDeploymentsErr := session.installDeployments(deployments); installDeploymentsErr != nil {
		return nil, installDeploymentsErr
	}
	engine, err := agent.NewEngine(agent.EngineConfig{
		DeploymentResolver:              deployments,
		ProcessAdmitter:                 agent.ProcessAdmitterFunc(session.admitProcess),
		ProcessStartOutcomeAcknowledger: agent.ProcessStartOutcomeAcknowledgerFunc(session.acknowledgeProcessStartOutcome),
		EventListeners:                  []agent.EventListener{agent.EventListenerFunc(session.observeFrameworkEvent)},
		DeltaListeners:                  []agent.DeltaListener{agent.DeltaListenerFunc(session.projectDelta)},
		DeltaBufferCapacity:             i.policy.deltaBufferCapacity,
		Limits:                          agent.DefaultLimits(),
		TreeLimits:                      deployments.treeLimits,
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: build Interaction engine: %w", err)
	}
	session.engine = engine
	return session, nil
}

func (i *InteractionExecutor) validateInteractionTools(manifest toolset.Manifest) error {
	if len(manifest.Visible)+len(manifest.Deferred) == 0 {
		return nil
	}
	if i.config.ToolInterpreter == nil {
		return errors.New("agentexec: Interaction Tools require a Tool interpreter")
	}
	if i.config.ToolAuthorizer == nil {
		return errors.New("agentexec: Interaction Tools require a Tool authorizer")
	}
	for _, tools := range [][]toolcontract.Tool{manifest.Visible, manifest.Deferred} {
		for _, executable := range tools {
			name := executable.Definition().Name
			if class := i.config.ToolInterpreter.SafetyClass(name); !class.Valid() {
				return fmt.Errorf(
					"agentexec: Interaction Tool %q has invalid safety class %q",
					name,
					class,
				)
			}
		}
	}
	return nil
}

func (i *InteractionExecutor) interactionConfiguration(
	session *interactionSession,
	start runs.RootExecutionStart,
	maxModelCalls uint32,
	manifest toolset.Manifest,
	group domaintool.Group,
	depth uint32,
	delegate agent.DeploymentRef,
	delegateBudget agent.Budget,
	instructions []corechat.Message,
) ([]byte, error) {
	configuration, err := json.Marshal(struct {
		Identity               string                     `json:"identity"`
		Provider               string                     `json:"provider"`
		Model                  string                     `json:"model"`
		MaxModelCalls          uint32                     `json:"maxModelCalls"`
		Streaming              bool                       `json:"streaming"`
		MaxConcurrentToolCalls int                        `json:"maxConcurrentToolCalls"`
		ToolResultOffload      *toolResultOffloadIdentity `json:"toolResultOffload,omitempty"`
		InteractiveApproval    bool                       `json:"interactiveApproval"`
		ContextCompaction      bool                       `json:"contextCompaction"`
		VisibleTools           []corechat.ToolDefinition  `json:"visibleTools,omitempty"`
		DeferredTools          []corechat.ToolDefinition  `json:"deferredTools,omitempty"`
		Group                  domaintool.Group           `json:"group"`
		Depth                  uint32                     `json:"depth"`
		Delegate               string                     `json:"delegate,omitempty"`
		DelegateBudget         agent.Budget               `json:"delegateBudget,omitzero"`
		Instructions           []corechat.Message         `json:"instructions,omitempty"`
	}{
		Identity: i.configurationIdentity.String(),
		Provider: session.accounting.providerName(), Model: session.accounting.modelName(),
		MaxModelCalls: maxModelCalls, Streaming: i.config.StreamModelResponses,
		MaxConcurrentToolCalls: i.policy.maxConcurrentToolCalls,
		ToolResultOffload:      i.policy.toolResultOffload.identity(),
		InteractiveApproval:    i.config.ToolAuthorizer != nil,
		ContextCompaction:      i.config.ModelContextCompactor != nil,
		VisibleTools:           toolDefinitions(manifest.Visible), DeferredTools: toolDefinitions(manifest.Deferred),
		Group: group, Depth: depth, Delegate: delegate.String(), DelegateBudget: delegateBudget,
		Instructions: cloneChatMessages(instructions),
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec: encode Interaction configuration identity: %w", err)
	}
	return configuration, nil
}

// BeginShutdown atomically rejects future roots. The remaining live set can
// only shrink; resource release is joined by AwaitShutdown under its caller's
// deadline so an interrupted close remains retryable.
func (i *InteractionExecutor) BeginShutdown() {
	if i == nil {
		return
	}
	i.sessions.closeAdmission()
}

// AwaitShutdown releases every root frozen by BeginShutdown. Successful
// targets are removed immediately; failed or timed-out targets stay owned for a
// later attempt.
func (i *InteractionExecutor) AwaitShutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("agentexec: Interaction shutdown context is required")
	}
	if i == nil {
		return nil
	}
	i.BeginShutdown()
	targets := i.sessions.snapshot()
	var failures []error
	for _, session := range targets {
		if err := session.release(ctx); err != nil {
			failures = append(failures, fmt.Errorf(
				"agentexec: release Interaction %q: %w",
				session.ref.ExecutorID,
				err,
			))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		i.sessions.remove(session)
	}
	return errors.Join(failures...)
}

func toolDefinitions(tools []toolcontract.Tool) []corechat.ToolDefinition {
	definitions := make([]corechat.ToolDefinition, len(tools))
	for index, executable := range tools {
		definitions[index] = executable.Definition().Clone()
	}
	return definitions
}

// Observe attaches the single Application Run pump before Process start.
// Streaming facts are best-effort; authoritative completion and termination
// are always emitted from Process.Await.
func (i *InteractionExecutor) Observe(
	ctx context.Context,
	ref runs.ExecutorRef,
) (iter.Seq[runs.ExecutorEvent], error) {
	session, err := i.session(ref)
	if err != nil {
		return nil, err
	}
	if !session.state.attachObserver() {
		return nil, errors.New("agentexec: Interaction execution already has an observer")
	}
	stopDetach := context.AfterFunc(ctx, session.state.detachObserver)
	return func(yield func(runs.ExecutorEvent) bool) {
		defer func() {
			stopDetach()
			session.state.detachObserver()
		}()
		for {
			select {
			case event, open := <-session.lifetime.events:
				if !open || !yield(event) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}, nil
}

// BeginRoot starts the staged Process exactly once. The accepted Run uses the
// staged session lifecycle rather than the caller's request context, so a client
// disconnect cannot cancel durable execution. Agent Framework owns every step;
// this adapter only awaits and translates the immutable terminal result.
func (i *InteractionExecutor) BeginRoot(_ context.Context, ref runs.ExecutorRef) error {
	session, err := i.session(ref)
	if err != nil {
		return err
	}
	if !session.state.begin() {
		return errors.New("agentexec: Interaction execution was already begun")
	}
	if !session.state.observerAttached() {
		session.failStart()
		return errors.New("agentexec: Interaction execution must be observed before begin")
	}
	process, err := session.engine.Start(
		runExecutionContext(session.lifetime.execution, session.scope, session.start),
		session.deployment,
		session.input,
	)
	if err != nil {
		session.failStart()
		return fmt.Errorf("agentexec: start Interaction: %w", err)
	}
	session.state.setProcess(process)
	session.startWorkers()
	return nil
}

// StageContinuation claims the exact process-local waiting boundary without
// advancing it. A missing live owner is rebuilt only from the supplied exact
// TreeSnapshot; a mismatch is rejected instead of silently recapturing state.
func (i *InteractionExecutor) StageContinuation(
	ctx context.Context,
	continuation runs.WaitingContinuation,
) (runs.ExecutorRef, error) {
	continuation = continuation.Clone()
	if err := continuation.Validate(); err != nil {
		return runs.ExecutorRef{}, err
	}
	if !i.acceptsBuild(continuation.Checkpoint.BuildID) {
		return runs.ExecutorRef{}, fmt.Errorf(
			"%w: checkpoint build %q does not match %q",
			runs.ErrExecutorStateLost,
			continuation.Checkpoint.BuildID,
			i.buildID.String(),
		)
	}
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	session, err := i.session(ref)
	if err == nil {
		if stageContinuationErr := session.stageContinuation(continuation.Checkpoint); stageContinuationErr != nil {
			return runs.ExecutorRef{}, stageContinuationErr
		}
		return ref, nil
	}
	if !errors.Is(err, runs.ErrExecutorNotLive) {
		return runs.ExecutorRef{}, err
	}
	if err := i.restoreWaitingTree(
		ctx,
		ref,
		continuation,
		interactionBoundaryContinuationStaged,
	); err != nil {
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

// RestoreWaitingExecution reconstructs an exact committed waiting tree without
// claiming it for continuation. An existing owner is rejected: recovery must
// first prove that the obsolete execution was released.
func (i *InteractionExecutor) RestoreWaitingExecution(
	ctx context.Context,
	continuation runs.WaitingContinuation,
) (runs.ExecutorRef, error) {
	continuation = continuation.Clone()
	if err := continuation.Validate(); err != nil {
		return runs.ExecutorRef{}, err
	}
	if !i.acceptsBuild(continuation.Checkpoint.BuildID) {
		return runs.ExecutorRef{}, fmt.Errorf(
			"%w: checkpoint build %q does not match %q",
			runs.ErrExecutorStateLost,
			continuation.Checkpoint.BuildID,
			i.buildID.String(),
		)
	}
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	if _, err := i.session(ref); err == nil {
		return runs.ExecutorRef{}, runs.ErrExecutionClaimed
	} else if !errors.Is(err, runs.ErrExecutorNotLive) {
		return runs.ExecutorRef{}, err
	}
	if err := i.restoreWaitingTree(
		ctx,
		ref,
		continuation,
		interactionBoundaryWaiting,
	); err != nil {
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

func (i *InteractionExecutor) restoreWaitingTree(
	ctx context.Context,
	ref runs.ExecutorRef,
	continuation runs.WaitingContinuation,
	boundary interactionBoundary,
) error {
	if err := i.validateRestoreScope(ctx, continuation.Checkpoint.Scope); err != nil {
		return err
	}
	checkpoint, err := decodeInteractionCheckpointPayload(continuation.Checkpoint.Payload)
	if err != nil {
		return fmt.Errorf("%w: parse Interaction checkpoint: %w", runs.ErrExecutorStateLost, err)
	}
	rootID, err := agent.ParseProcessID(continuation.Checkpoint.RootMemberID)
	if err != nil || checkpoint.tree.RootID() != rootID {
		return fmt.Errorf("%w: checkpoint root differs from its tree", runs.ErrExecutorStateLost)
	}
	processSnapshots := checkpoint.tree.ProcessSnapshots()
	if len(processSnapshots) == 0 || processSnapshots[0].ProcessID() != rootID ||
		!isInteractionWaitingBoundary(processSnapshots[0].Status()) {
		return fmt.Errorf("%w: Interaction restore requires a product waiting boundary", runs.ErrExecutorStateLost)
	}
	start := runs.RootExecutionStart{
		SessionID: continuation.SessionID,
		CWD:       continuation.Checkpoint.Scope.CWD, WorkspaceCWD: continuation.Checkpoint.Scope.WorkspaceCWD,
		Isolated: continuation.Checkpoint.Scope.Isolated, GoalIncarnationID: continuation.Checkpoint.Scope.GoalIncarnationID,
		ModelSelection: continuation.Checkpoint.ModelSelection, Limits: continuation.Checkpoint.Limits,
		InterruptKinds:           continuation.Capabilities.InterruptKinds,
		ChildRunAdmissionEnabled: continuation.ChildRunAdmissionEnabled,
		WorkingContext:           cloneChatMessages(checkpoint.instructions),
	}
	session, err := i.assembleInteraction(ctx, ref, start)
	if err != nil {
		return err
	}
	process, err := session.engine.RestoreTree(
		runExecutionContext(session.lifetime.execution, session.scope, session.start),
		session.deployment,
		checkpoint.tree,
	)
	if err != nil {
		_ = session.engine.Close()
		return fmt.Errorf("%w: restore exact Interaction tree: %w", runs.ErrExecutorStateLost, err)
	}
	if initializeRestoredContinuationErr := session.initializeRestoredContinuation(process, continuation, checkpoint, boundary); initializeRestoredContinuationErr != nil {
		discardRestoredInteraction(session, process)
		return initializeRestoredContinuationErr
	}
	unknown, err := session.unknownEffectIDs(ctx)
	if err != nil {
		discardRestoredInteraction(session, process)
		return fmt.Errorf("%w: inspect restored Interaction effects: %v", runs.ErrExecutorStateLost, err)
	}
	if len(unknown) > 0 {
		discardRestoredInteraction(session, process)
		return fmt.Errorf("%w: restored Interaction has unresolved effects", runs.ErrExecutorStateLost)
	}
	interruptions, err := session.pendingInterruptions(checkpoint.tree)
	if err != nil || len(interruptions) == 0 {
		discardRestoredInteraction(session, process)
		if err != nil {
			return fmt.Errorf("%w: inspect restored Interaction input: %v", runs.ErrExecutorStateLost, err)
		}
		return fmt.Errorf("%w: restored Interaction tree has no pending input", runs.ErrExecutorStateLost)
	}
	if err := i.sessions.register(session); err != nil {
		discardRestoredInteraction(session, process)
		return err
	}
	session.startWorkers()
	return nil
}

func (i *InteractionExecutor) validateRestoreScope(
	ctx context.Context,
	scope runs.ExecutionScope,
) error {
	if scope.Isolated {
		return fmt.Errorf("%w: isolated workspaces are not restorable after executor loss", runs.ErrExecutorStateLost)
	}
	if i.config.RestoreScopeValidator != nil {
		if err := i.config.RestoreScopeValidator.ValidateRestoreScope(ctx, scope); err != nil {
			return fmt.Errorf("%w: validate restore scope: %v", runs.ErrExecutorStateLost, err)
		}
		return nil
	}
	for _, path := range []string{scope.CWD, scope.WorkspaceCWD} {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%w: restore workspace path is empty", runs.ErrExecutorStateLost)
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("%w: restore workspace %q is unavailable", runs.ErrExecutorStateLost, path)
		}
	}
	return nil
}

func discardRestoredInteraction(session *interactionSession, process *agent.Process) {
	cleanupCtx, cancel := context.WithTimeout(
		context.WithoutCancel(session.lifetime.execution),
		authoritativeProjectionTimeout,
	)
	defer cancel()
	_ = process.Kill(cleanupCtx, interactionReleaseReason)
	_, _ = process.Await(cleanupCtx)
	_ = session.engine.Close()
}

func rootExecutionScope(start runs.RootExecutionStart) runs.ExecutionScope {
	return runs.ExecutionScope{
		SessionID: start.SessionID, CWD: start.CWD, WorkspaceCWD: start.WorkspaceCWD,
		Isolated: start.Isolated, GoalIncarnationID: start.GoalIncarnationID,
	}
}

func runExecutionContext(
	ctx context.Context,
	scope runs.ExecutionScope,
	start runs.RootExecutionStart,
) context.Context {
	capabilities := run.Capabilities{
		ChildRuns:      start.ChildRunAdmissionEnabled,
		InterruptKinds: slices.Clone(start.InterruptKinds),
	}.Normalized()
	ctx = executionctx.WithScope(ctx, scope)
	ctx = executionctx.WithRunCapabilities(ctx, capabilities)
	return executionctx.WithModelSelection(ctx, start.ModelSelection)
}

// Release tears down one staged or terminal per-root Engine. It is idempotent
// and does not decide the product Run outcome.
func (i *InteractionExecutor) Release(ctx context.Context, ref runs.ExecutorRef) error {
	if i == nil {
		return nil
	}
	session, err := i.sessions.lookup(ref)
	if err != nil {
		return err
	}
	if session == nil {
		return nil
	}

	err = session.release(ctx)
	if err == nil {
		i.sessions.remove(session)
	}
	return err
}

// RequestRootCancellation submits the Application's accepted cancellation to
// Agent Framework without deciding the product outcome or releasing the tree.
// Success means the request entered Engine's queue. The adapter then cancels
// its cooperative in-flight model/Tool dispatches so they can settle promptly;
// Agent Framework remains the sole lifecycle owner and applies the accepted
// intent only after that safe settlement boundary.
func (i *InteractionExecutor) RequestRootCancellation(
	ctx context.Context,
	ref runs.ExecutorRef,
	reason string,
) error {
	session, err := i.session(ref)
	if err != nil {
		return err
	}
	process := session.state.processHandle()
	if process == nil {
		return runs.ErrExecutorNotLive
	}
	if err := process.RequestCancellation(ctx, reason); err != nil &&
		!errors.Is(err, agent.ErrProcessFinished) {
		return fmt.Errorf("agentexec: submit Interaction cancellation intent: %w", err)
	}
	session.cancelAllDispatches()
	return nil
}

func (i *InteractionExecutor) resolveChat(
	ctx context.Context,
	selection modelref.Selection,
) (modeladapter.ResolvedChat, error) {
	if err := selection.ValidateExact(); err != nil {
		return modeladapter.ResolvedChat{}, fmt.Errorf("agentexec: Interaction: %w", err)
	}
	resolved, err := i.config.ChatResolver.ResolveChat(ctx, selection)
	if err != nil {
		return modeladapter.ResolvedChat{}, fmt.Errorf("agentexec: resolve Interaction chat: %w", err)
	}
	if resolved.Client() == nil {
		return modeladapter.ResolvedChat{}, errors.New("agentexec: Interaction chat resolver returned an invalid result")
	}
	return resolved, nil
}

func (i *InteractionExecutor) maxModelCalls(start runs.RootExecutionStart) (uint32, error) {
	maxSteps, limited := start.Limits.MaxSteps()
	if !limited {
		return i.policy.defaultMaxModelCalls, nil
	}
	if uint64(maxSteps) > math.MaxUint32 {
		return 0, fmt.Errorf("%w: max steps exceeds Interaction model-call range", runs.ErrInvalidRunLimit)
	}
	return uint32(maxSteps), nil
}

func (i *InteractionExecutor) session(ref runs.ExecutorRef) (*interactionSession, error) {
	if i == nil {
		return nil, errors.New("agentexec: Interaction executor is nil")
	}
	return i.sessions.require(ref)
}

func cloneChatMessages(messages []corechat.Message) []corechat.Message {
	cloned := make([]corechat.Message, len(messages))
	for index := range messages {
		cloned[index] = messages[index].Clone()
	}
	return cloned
}

func clonedOptions(options *corechat.Options) corechat.Options {
	if options == nil {
		return corechat.Options{}
	}
	return options.Clone()
}

func executionOptions(selection modelref.Selection, options *corechat.Options) corechat.Options {
	cloned := clonedOptions(options)
	cloned.ReasoningEffort = corechat.ReasoningEffort(selection.ReasoningEffort())
	return cloned
}

var (
	_ runs.RootExecutionStarter             = (*InteractionExecutor)(nil)
	_ runs.ExecutionObserver                = (*InteractionExecutor)(nil)
	_ runs.ExecutionReleaser                = (*InteractionExecutor)(nil)
	_ runs.RunningRootCancellationRequester = (*InteractionExecutor)(nil)
	_ runs.WaitingExecutionContinuer        = (*InteractionExecutor)(nil)
	_ runs.RunningExecutionSteerer          = (*InteractionExecutor)(nil)
)
