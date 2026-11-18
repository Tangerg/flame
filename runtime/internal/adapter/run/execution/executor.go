package execution

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"slices"
	"strings"
	"time"

	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	otelagent "github.com/Tangerg/scope/otel/agent"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

const (
	interactionDefinitionName        = "flame.runtime.interaction"
	interactionDefinitionDescription = "Run one model-directed Flame interaction over a frozen working context."
	delegatedDefinitionDescription   = "Run one isolated delegated Flame interaction."
	interactionEventBuffer           = 64
	interactionReleaseReason         = "runtime released execution resources"
	defaultUnknownEffectPollInterval = time.Second
	defaultInteractionStatePoll      = 250 * time.Millisecond
)

// InteractionChatResolver resolves one exact provider construction selected by
// a Run. Resolution happens during staging and must not invoke the model.
type InteractionChatResolver interface {
	ResolveChat(ctx context.Context, selection modelchoice.Selection) (modeladapter.ResolvedChat, error)
	RestoreChat(context.Context, modelchoice.Selection, *modeladapter.ProviderCapture) (modeladapter.ResolvedChat, error)
}

type interactionModelEvidence interface {
	EndpointForCaller(context.Context, string) (modelref.EndpointObservation, bool, error)
}

type executionJournal interface {
	runs.ExecutionTreeStore
	LoadCheckpoint(ctx context.Context, rootMemberID string) (runs.ExecutorCheckpoint, error)
}

type originalToolResults interface {
	OwnsToolSource(context.Context, domaintool.Ref, framevalue.Digest, plugin.Activation) (bool, error)
	AwaitOriginalToolSource(context.Context, domaintool.Ref, framevalue.Digest, framevalue.Digest, plugin.Activation) (bool, error)
	OriginalToolResult(context.Context, domaintool.Ref, framevalue.Digest, framevalue.Digest, plugin.Activation, string, string) (*toolcontract.Output, error)
}

// InteractionExecutorConfig freezes the host-owned inputs shared by
// Interaction root executions. Identity strings must change whenever the
// executable Interaction adapter or behavior-affecting dispatcher configuration
// changes, so Agent Framework Deployment references remain honest.
type InteractionExecutorConfig struct {
	Admission framehost.Admission
	// ValidatePluginSources rechecks captured plans and package releases under the
	// original shared admission before the execution becomes a live use owner.
	ValidatePluginSources func(context.Context, []plugin.Activation, []plugin.Dependency) error
	// Lifetime is the process-owned root for every Interaction staged by this
	// executor. Request contexts may bound staging and commands, but accepted
	// execution must outlive the request that created it.
	Lifetime                  context.Context
	ExecutionTrees            executionJournal
	OriginalToolResults       originalToolResults
	BuildID                   string
	ChatResolver              InteractionChatResolver
	ModelEvidence             interactionModelEvidence
	CallComposition           interactionCallComposition
	StreamModelResponses      bool
	DeltaBufferCapacity       *int
	MaxConcurrentToolCalls    *int
	ToolResolver              interactionToolResolver
	ToolInterpreter           interactionToolInterpreter
	ToolPresenter             interactionToolPresenter
	ToolAuthorizer            interactionToolAuthorizer
	ToolHooks                 interactionToolHooks
	Maintenance               RunMaintenance
	ModelContextCompactor     ModelContextCompactor
	ModelContextState         interactionModelContextState
	LifecycleHooks            interactionLifecycleHooks
	ToolResultOffload         *ToolResultOffload
	UnknownEffectPollInterval *time.Duration
	StatePollInterval         *time.Duration
}

// InteractionExecutor is the Agent Framework root execution adapter. Each staged
// root owns an independent Engine and exactly one Interaction Process; the
// Application owns durable Run state and consumes only [runs.ExecutorEvent].
type InteractionExecutor struct {
	installations framehost.Admission
	lifetime      context.Context
	config        InteractionExecutorConfig
	policy        interactionExecutionPolicy
	buildID       runs.BuildID

	sessions interactionSessions
}

// NewInteractionExecutor validates immutable host configuration. It creates no
// Engine, goroutine, model call, or tool call; those resources are per staged
// root execution.
func NewInteractionExecutor(config InteractionExecutorConfig) (*InteractionExecutor, error) {
	if config.Lifetime == nil {
		return nil, errors.New("execution: Interaction lifetime is required")
	}
	if lo.IsNil(config.ChatResolver) {
		return nil, errors.New("execution: Interaction requires a chat resolver")
	}
	if lo.IsNil(config.ModelEvidence) {
		return nil, errors.New("execution: Interaction model evidence is required")
	}
	if config.ValidatePluginSources == nil {
		return nil, errors.New("execution: plugin source validation is required")
	}
	if !config.Admission.Valid() {
		return nil, errors.New("execution: installation admission is required")
	}
	if lo.IsNil(config.ExecutionTrees) {
		return nil, errors.New("execution: execution tree store is required")
	}
	if lo.IsNil(config.ModelContextCompactor) != lo.IsNil(config.ModelContextState) {
		return nil, errors.New("execution: model-context compactor and state source must be configured together")
	}
	for _, capability := range []struct {
		name  string
		value any
	}{
		{name: "chat resolver", value: config.ChatResolver},
		{name: "model composition", value: config.CallComposition},
		{name: "original Tool results", value: config.OriginalToolResults},
		{name: "Tool resolver", value: config.ToolResolver},
		{name: "Tool interpreter", value: config.ToolInterpreter},
		{name: "Tool presenter", value: config.ToolPresenter},
		{name: "Tool authorizer", value: config.ToolAuthorizer},
		{name: "Tool hooks", value: config.ToolHooks},
		{name: "Run maintenance", value: config.Maintenance},
		{name: "model-context compactor", value: config.ModelContextCompactor},
		{name: "model-context state", value: config.ModelContextState},
		{name: "lifecycle hooks", value: config.LifecycleHooks},
	} {
		if capability.value != nil && lo.IsNil(capability.value) {
			return nil, fmt.Errorf("execution: Interaction %s is typed nil", capability.name)
		}
	}
	buildID, err := runs.ParseBuild(config.BuildID)
	if err != nil {
		return nil, fmt.Errorf("execution: Interaction %w", err)
	}
	policy, err := newInteractionExecutionPolicy(config)
	if err != nil {
		return nil, err
	}
	lifetime := config.Lifetime
	config.Lifetime = nil
	config.BuildID = ""
	return &InteractionExecutor{
		lifetime: lifetime, config: config, policy: policy,
		buildID:       buildID,
		sessions:      newInteractionSessions(),
		installations: config.Admission,
	}, nil
}

func (i *InteractionExecutor) acceptsBuild(raw string) bool {
	identity, err := runs.ParseBuild(raw)
	return err == nil && identity == i.buildID
}

func interactionStartID(sessionID, runID string) scoperun.StartID {
	return scoperun.StartID{Scope: sessionID, Key: runID}
}

func newExecutorRef(sessionID string) runs.ExecutorRef {
	return runs.ExecutorRef{SessionID: sessionID, ExecutorID: "exec_" + uuid.NewString()}
}

func validateModelOutputReservation(
	limits modelref.TokenLimits,
	options *corechat.Options,
) error {
	if options == nil || options.MaxOutputTokens == nil {
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
) (_ runs.ExecutorRef, err error) {
	finishAssembly, err := i.sessions.beginAssembly()
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	defer finishAssembly()
	start = start.Clone()
	if err := flameidentity.ValidateSession(start.SessionID); err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("execution: Interaction: %w", err)
	}
	if err := flameidentity.ValidateRun(start.RunID); err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("execution: Interaction: %w", err)
	}
	if !interactionStartID(start.SessionID, start.RunID).Valid() {
		return runs.ExecutorRef{}, fmt.Errorf("%w: invalid Scope start identity", runs.ErrInvalidStart)
	}
	if err := start.Validate(); err != nil {
		return runs.ExecutorRef{}, err
	}
	ref := newExecutorRef(start.SessionID)
	session, err := i.assembleInteraction(ctx, ref, start, nil)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, i.discardInteraction(session))
		}
	}()
	input, err := scoperun.EncodePayload(interaction.Input{
		Messages: cloneChatMessages(start.WorkingContext), Options: executionOptions(start.ModelSelection, start.Options),
	})
	if err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("execution: encode Interaction input: %w", err)
	}
	session.input = input
	if err := i.publishInteraction(ctx, session); err != nil {
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

var (
	errProposalCompositionUnavailable = errors.New("execution: captured Tool proposal composition is unavailable")
	errModelInputUnavailable          = errors.New("execution: captured model input is unavailable")
	errModelContextUnavailable        = errors.New("execution: captured model context is unavailable")
	errModelObserversUnavailable      = errors.New("execution: captured model observers are unavailable")
)

// captureComposition fixes the call composition a Run executes with. A
// restored checkpoint replays its original capture instead of resolving the
// current composition again.
func (i *InteractionExecutor) captureComposition(ctx context.Context, session *interactionSession, checkpoint *interactionCheckpointState) error {
	if checkpoint != nil {
		session.modelObservers = slices.Clone(checkpoint.modelObservers)
		session.modelContexts = slices.Clone(checkpoint.modelContexts)
		session.modelInputs = slices.Clone(checkpoint.modelInputs)
		session.toolProposals = slices.Clone(checkpoint.toolProposals)
		return nil
	}
	composition := i.config.CallComposition
	if composition == nil {
		return nil
	}
	var err error
	if session.modelObservers, err = composition.Resolve(ctx); err != nil {
		return err
	}
	if session.modelContexts, err = composition.ResolveContext(ctx); err != nil {
		return err
	}
	if session.modelInputs, err = composition.ResolveInput(ctx); err != nil {
		return err
	}
	session.toolProposals, err = composition.ResolveToolProposals(ctx)
	return err
}

// validateCaptured revalidates a captured composition member against the live
// composition; a capture this Runtime can no longer compose is refused.
func (i *InteractionExecutor) validateCaptured(captured bool, unavailable error, validate func() error) error {
	if !captured {
		return nil
	}
	if i.config.CallComposition == nil {
		return unavailable
	}
	return validate()
}

func (i *InteractionExecutor) assembleInteraction(
	ctx context.Context,
	ref runs.ExecutorRef,
	start runs.RootExecutionStart,
	checkpoint *interactionCheckpointState,
) (_ *interactionSession, err error) {
	var resolved modeladapter.ResolvedChat
	if checkpoint != nil {
		resolved, err = i.config.ChatResolver.RestoreChat(ctx, start.ModelSelection, checkpoint.modelProvider)
	} else {
		resolved, err = i.resolveChat(ctx, start.ModelSelection)
	}
	if err != nil {
		return nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			err = errors.Join(err, resolved.Close(context.WithoutCancel(ctx)))
		}
	}()
	model := resolved.Model()
	streamer, _ := resolved.Streamer()
	counter, _ := resolved.InputTokenCounter()
	telemetry, err := otelagent.NewObserver(otelagent.ObserverConfig{})
	if err != nil {
		return nil, fmt.Errorf("execution: observe Interaction execution: %w", err)
	}
	selected, err := modeladapter.NewSelectedChat(start.ModelSelection, resolved)
	if err != nil {
		return nil, err
	}
	if err := validateModelOutputReservation(selected.TokenLimits(), start.Options); err != nil {
		return nil, err
	}
	session := newInteractionSession(i.lifetime, ref, start, i.config, i.buildID, i.policy, telemetry)
	session.selectedChat = selected
	transferred = true
	i.sessions.own(session)
	defer func() {
		if err != nil {
			err = errors.Join(err, i.discardInteraction(session))
		}
	}()
	if err := i.captureComposition(ctx, session, checkpoint); err != nil {
		return nil, err
	}
	composition := i.config.CallComposition
	if err := i.validateCaptured(len(session.toolProposals) > 0, errProposalCompositionUnavailable, func() error {
		return composition.ValidateToolProposals(ctx, session.toolProposals)
	}); err != nil {
		return nil, err
	}
	if composition != nil {
		var activations []plugin.Activation
		if checkpoint != nil {
			activations = checkpoint.bindings
		} else if activations, err = composition.Activations(ctx, session.compositionBindings()); err != nil {
			return nil, err
		}
		if err := session.bindCompositionActivations(activations); err != nil {
			return nil, err
		}
	}
	if err := i.validateCaptured(len(session.modelInputs) > 0, errModelInputUnavailable, func() error {
		return composition.ValidateInput(ctx, session.modelInputs)
	}); err != nil {
		return nil, err
	}
	if err := i.validateCaptured(len(session.modelContexts) > 0, errModelContextUnavailable, func() error {
		return composition.ValidateContext(ctx, session.modelContexts)
	}); err != nil {
		return nil, err
	}
	if err := i.validateCaptured(len(session.modelObservers) > 0, errModelObserversUnavailable, func() error {
		return composition.Validate(ctx, session.modelObservers)
	}); err != nil {
		return nil, err
	}
	if len(session.modelObservers) > 0 {
		model, streamer = composition.Bind(model, streamer, session.modelObservers)
	}
	observed, err := newObservedInteractionModel(model, streamer, session)
	if err != nil {
		return nil, fmt.Errorf("execution: observe Interaction model: %w", err)
	}
	store, err := telemetry.WrapStore(session)
	if err != nil {
		return nil, fmt.Errorf("execution: observe Interaction store: %w", err)
	}
	session.deltas = make(chan scoperun.Delta, i.policy.deltaBufferCapacity)
	session.engine, err = scoperun.New(session.lifetime.execution, scoperun.Config{
		Store: store, Build: i.buildID.String(),
		ProcessAdmitter: scoperun.ProcessAdmitterFunc(session.admitProcess),
		Deltas:          session.deltas, TreeLimits: interactionTreeLimits(start),
	})
	if err != nil {
		return nil, fmt.Errorf("execution: build Interaction Runtime: %w", err)
	}
	deployments, err := i.buildInteractionDeployments(
		runExecutionContext(ctx, rootExecutionScope(start), start), session, start, observed, counter, checkpoint,
	)
	if err != nil {
		return nil, err
	}
	if installDeploymentsErr := session.installDeployments(deployments); installDeploymentsErr != nil {
		return nil, errors.Join(installDeploymentsErr, deployments.close())
	}
	return session, nil
}

// The Deployment's configuration digest names what makes two executions
// interchangeable. The mutable model history stays outside this identity;
// static Tool and Delegate bindings enter it through Scope's ChildDeployments.
func (i *InteractionExecutor) interactionConfiguration(
	session *interactionSession,
	group domaintool.Group,
	depth uint32,
	instructions []corechat.Message,
) ([]byte, error) {
	configuration, err := scoperun.EncodePayload(struct {
		Provider               string                    `json:"provider"`
		Model                  string                    `json:"model"`
		Streaming              bool                      `json:"streaming"`
		MaxConcurrentToolCalls int                       `json:"maxConcurrentToolCalls"`
		ContextCompaction      bool                      `json:"contextCompaction"`
		Group                  domaintool.Group          `json:"group"`
		Depth                  uint32                    `json:"depth"`
		DelegateOptions        corechat.Options          `json:"delegateOptions"`
		Instructions           []corechat.Message        `json:"instructions,omitempty"`
		ModelObservers         []contributionBindingWire `json:"modelObservers"`
		CompositionActivations []string                  `json:"compositionActivations"`
		ModelContexts          []contributionBindingWire `json:"modelContexts"`
		ModelInputs            []contributionBindingWire `json:"modelInputs"`
		ToolProposals          []contributionBindingWire `json:"toolProposals"`
	}{
		Provider: session.accounting.providerName(), Model: session.accounting.modelName(),
		Streaming:              i.config.StreamModelResponses,
		MaxConcurrentToolCalls: i.policy.maxConcurrentToolCalls,
		ContextCompaction:      i.config.ModelContextCompactor != nil,
		Group:                  group, Depth: depth,
		Instructions: cloneChatMessages(instructions), DelegateOptions: delegatedExecutionOptions(session.start),
		ModelObservers:         contributionBindingsWire(session.modelObservers),
		CompositionActivations: activationIdentities(session.compositionActivations),
		ModelContexts:          contributionBindingsWire(session.modelContexts),
		ModelInputs:            contributionBindingsWire(session.modelInputs),
		ToolProposals:          contributionBindingsWire(session.toolProposals),
	})
	if err != nil {
		return nil, fmt.Errorf("execution: encode Interaction configuration identity: %w", err)
	}
	return configuration.JSON(), nil
}

func (i *InteractionExecutor) interactionToolConfiguration(manifest toolset.Manifest, observed map[domaintool.Ref]*observedInteractionTool) ([]byte, error) {
	visible, err := toolIdentities(manifest.Visible, observed)
	if err != nil {
		return nil, err
	}
	deferred, err := toolIdentities(manifest.Deferred, observed)
	if err != nil {
		return nil, err
	}
	// Deferred authority is a set; discovery owns its presentation order.
	// Reconnection order must not change a waiting execution's identity.
	slices.SortFunc(deferred, func(a, b toolConfigurationIdentity) int {
		return strings.Compare(a.Reference, b.Reference)
	})
	configuration, err := scoperun.EncodePayload(struct {
		Installations     []installationDependencyWire `json:"installations"`
		ToolResultOffload *toolResultOffloadIdentity   `json:"toolResultOffload,omitzero"`
		ToolHooks         bool                         `json:"toolHooks"`
		VisibleTools      []toolConfigurationIdentity  `json:"visibleTools,omitempty"`
		DeferredTools     []toolConfigurationIdentity  `json:"deferredTools,omitempty"`
	}{installationDependencies(manifest.Installations), i.policy.toolResultOffload.identity(), i.config.ToolHooks != nil, visible, deferred})
	if err != nil {
		return nil, fmt.Errorf("execution: encode Interaction Tool configuration identity: %w", err)
	}
	return configuration.JSON(), nil
}

type toolConfigurationIdentity struct {
	Definition        toolcontract.Definition `json:"definition"`
	Reference         string                  `json:"reference"`
	SourceFingerprint string                  `json:"sourceFingerprint,omitempty"`
	Activation        string                  `json:"activation,omitempty"`
}

func toolIdentities(tools []toolcontract.Tool, observed map[domaintool.Ref]*observedInteractionTool) ([]toolConfigurationIdentity, error) {
	identities := make([]toolConfigurationIdentity, 0, len(tools))
	for _, executable := range tools {
		ref, err := toolset.Identify(executable)
		if err != nil {
			return nil, err
		}
		owner := observed[ref]
		if owner == nil {
			return nil, fmt.Errorf("execution: Tool %s has no admitted contract", ref)
		}
		activation, err := toolset.InstallationActivation(owner.inner)
		if err != nil {
			return nil, err
		}
		identities = append(identities, toolConfigurationIdentity{Definition: owner.binding.Contract().Definition(), Reference: ref.String(), SourceFingerprint: owner.sourceFingerprint.String(), Activation: activation.Identity.String()})
	}
	return identities, nil
}

// BeginShutdown rejects new assembly and publication. AwaitShutdown joins
// admitted assembly and releases its resources under the caller's deadline.
func (i *InteractionExecutor) BeginShutdown() {
	i.sessions.closeAdmission()
}

// AwaitShutdown releases published roots, probes, and failed assemblies.
// Successful targets are removed; failed or timed-out targets stay owned.
func (i *InteractionExecutor) AwaitShutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("execution: Interaction shutdown context is required")
	}
	i.BeginShutdown()
	if err := i.sessions.awaitAssembly(ctx); err != nil {
		return err
	}
	targets := i.sessions.snapshot()
	var failures []error
	for _, session := range targets {
		if err := session.release(ctx); err != nil {
			failures = append(failures, fmt.Errorf(
				"execution: release Interaction %q: %w",
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
	detach, attached := session.state.attachObserver(ctx)
	if !attached {
		return nil, errors.New("execution: Interaction execution already has an observer")
	}
	continuedTools := session.continuedDelegateTools()
	stopDetach := context.AfterFunc(ctx, detach)
	return func(yield func(runs.ExecutorEvent) bool) {
		defer func() {
			stopDetach()
			detach()
		}()
		// The fresh Segment's reducer must know every continued Delegate before
		// it consumes buffered child results or new execution facts.
		for _, event := range continuedTools {
			if ctx.Err() != nil || !yield(event) {
				return
			}
		}
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
	if err := session.state.begin(interactionBoundaryInactive); err != nil {
		return err
	}
	process, err := session.engine.Start(
		runExecutionContext(session.lifetime.execution, session.scope, session.start),
		session.deployment,
		scoperun.StartRequest[scoperun.Payload]{ID: interactionStartID(session.start.SessionID, session.start.RunID), Input: session.input},
	)
	if err != nil {
		session.failStart()
		return fmt.Errorf("execution: start Interaction: %w", err)
	}
	session.state.setProcess(process)
	session.startWorkers()
	return nil
}

// RestoreWaitingExecution reconstructs an exact committed waiting tree without
// claiming it for continuation. An existing owner is rejected: recovery must
// first prove that the obsolete execution was released.
func (i *InteractionExecutor) RestoreWaitingExecution(
	ctx context.Context,
	continuation runs.ExecutionContinuation,
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
	continuation runs.ExecutionContinuation,
	boundary interactionBoundary,
) (err error) {
	finishAssembly, err := i.sessions.beginAssembly()
	if err != nil {
		return err
	}
	defer finishAssembly()
	if err := validateRestoreWorkspace(continuation); err != nil {
		return err
	}
	checkpoint, err := decodeRunCheckpoint(continuation.Checkpoint, continuation.SessionID, continuation.RootRunID)
	if err != nil {
		return fmt.Errorf("%w: parse Interaction checkpoint: %w", runs.ErrExecutorStateLost, err)
	}
	rootID := checkpoint.handoff.rootID
	head, found, err := i.config.ExecutionTrees.LoadExecutionTree(ctx, continuation.SessionID, rootID.String())
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: execution tree head is missing", runs.ErrExecutorStateLost)
	}
	checkpoint.tree, err = decodeExecutionTree(head, rootID)
	if err != nil {
		return fmt.Errorf("%w: %w", runs.ErrExecutorStateLost, err)
	}
	inspection, err := checkpoint.tree.Inspect()
	if err != nil {
		return err
	}
	processSnapshots := inspection.Processes
	if len(processSnapshots) == 0 || processSnapshots[0].ID != rootID ||
		!isInteractionWaitingBoundary(processSnapshots[0].Status) {
		return fmt.Errorf("%w: Interaction restore requires a product waiting boundary", runs.ErrExecutorStateLost)
	}
	start := checkpoint.restoredStart(continuation)
	session, err := i.assembleInteraction(ctx, ref, start, &checkpoint)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, i.discardInteraction(session))
		}
	}()
	if pendingCheckpointSources(session, checkpoint) {
		return errors.New("execution: required MCP source is still connecting")
	}
	if err := session.validateWaitingTree(ctx, continuation, checkpoint, boundary == interactionBoundaryAcceptedPaused); err != nil {
		return fmt.Errorf("execution: validate waiting tree: %w", err)
	}
	// Recover commits activation without replaying process admission. Its first
	// Store callback must already see the original product ownership projection.
	if err := session.restoreContinuationProjection(continuation, checkpoint, boundary); err != nil {
		return err
	}
	process, err := session.engine.Recover(
		runExecutionContext(session.lifetime.execution, session.scope, session.start),
		session.deployment,
		checkpoint.tree.Root,
	)
	if err != nil {
		return fmt.Errorf("execution: restore exact Interaction tree: %w", err)
	}
	session.state.setProcess(process)
	session.state.mu.Lock()
	session.state.restoreTree = scoperun.Log{}
	session.state.mu.Unlock()
	unknown, err := session.unknownEffectIDs(ctx)
	if err != nil {
		return fmt.Errorf("execution: inspect restored Interaction tree: %w", err)
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%w: restored Interaction has unresolved effects", runs.ErrExecutorStateLost)
	}
	interruptions, err := session.pendingInterruptions(checkpoint.tree)
	if err != nil || (len(interruptions) == 0 && boundary != interactionBoundaryAcceptedPaused) {
		if err != nil {
			return fmt.Errorf("%w: inspect restored Interaction input: %w", runs.ErrExecutorStateLost, err)
		}
		return fmt.Errorf("%w: restored Interaction tree has no pending input", runs.ErrExecutorStateLost)
	}
	if err := i.publishInteraction(ctx, session); err != nil {
		return err
	}
	session.startWorkers()
	return nil
}

func validateRestoreWorkspace(continuation runs.ExecutionContinuation) error {
	if continuation.Isolated {
		return fmt.Errorf("%w: isolated workspaces are not restorable after executor loss", runs.ErrExecutorStateLost)
	}
	path := continuation.Workspace
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: restore workspace %q: %w", runs.ErrExecutorStateLost, path, err)
		}
		return fmt.Errorf("execution: inspect restore workspace %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: restore workspace %q is unavailable", runs.ErrExecutorStateLost, path)
	}
	return nil
}

func (i *InteractionExecutor) discardInteraction(session *interactionSession) error {
	cleanupCtx, cancel := context.WithTimeout(
		context.WithoutCancel(session.lifetime.execution),
		executionCleanupTimeout,
	)
	defer cancel()
	if err := session.release(cleanupCtx); err != nil {
		return fmt.Errorf("execution: discard Interaction %q: %w", session.ref.ExecutorID, err)
	}
	i.sessions.remove(session)
	return nil
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
		InterruptTypes: slices.Clone(start.InterruptTypes),
	}.Normalized()
	ctx = executionctx.WithScope(ctx, scope)
	return executionctx.WithRunCapabilities(ctx, capabilities)
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
// Success acknowledges submission only; Scope owns cancellation propagation and
// the immutable result reports whether cancellation became the terminal cause.
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
	if err := process.Cancel(ctx, scopeCancellationReason(reason)); err != nil &&
		!errors.Is(err, scoperun.ErrProcessFinished) {
		// A completed Runtime may already have closed its local writer. Only a
		// committed Exit, never the writer's closed state, proves this is late.
		inspection, inspectErr := session.inspectCommittedTree(ctx)
		if inspectErr == nil {
			if root, found := inspection.Process(process.ID()); found && root.Status.Terminal() {
				return nil
			}
		}
		return fmt.Errorf("execution: submit Interaction cancellation intent: %w", err)
	}
	return nil
}

func (i *InteractionExecutor) resolveChat(
	ctx context.Context,
	selection modelchoice.Selection,
) (modeladapter.ResolvedChat, error) {
	if err := selection.ValidateExact(); err != nil {
		return modeladapter.ResolvedChat{}, fmt.Errorf("execution: Interaction: %w", err)
	}
	resolved, err := i.config.ChatResolver.ResolveChat(ctx, selection)
	if err != nil {
		return modeladapter.ResolvedChat{}, fmt.Errorf("execution: resolve Interaction chat: %w", err)
	}
	if lo.IsNil(resolved.Model()) {
		return modeladapter.ResolvedChat{}, errors.New("execution: Interaction chat resolver returned an invalid result")
	}
	return resolved, nil
}

func (i *InteractionExecutor) session(ref runs.ExecutorRef) (*interactionSession, error) {
	if i == nil {
		return nil, errors.New("execution: Interaction executor is nil")
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

func executionOptions(selection modelchoice.Selection, options *corechat.Options) corechat.Options {
	cloned := clonedOptions(options)
	cloned.ReasoningEffort = corechat.ReasoningEffort(selection.ReasoningEffort())
	return cloned
}

var (
	_ runs.RootExecutionStarter             = (*InteractionExecutor)(nil)
	_ runs.ExecutionObserver                = (*InteractionExecutor)(nil)
	_ runs.ExecutionReleaser                = (*InteractionExecutor)(nil)
	_ runs.RunningRootCancellationRequester = (*InteractionExecutor)(nil)
	_ runs.ExecutionContinuer               = (*InteractionExecutor)(nil)
	_ runs.RunningExecutionSteerer          = (*InteractionExecutor)(nil)
)

// Delegates inherit generation policy, but root output framing belongs only to
// the root task. The deployment digest binds this policy across restoration.
func delegatedExecutionOptions(start runs.RootExecutionStart) corechat.Options {
	options := executionOptions(start.ModelSelection, start.Options)
	options.OutputFormat = nil
	options.Stop = nil
	return options
}
