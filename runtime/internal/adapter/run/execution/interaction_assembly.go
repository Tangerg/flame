package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	otelagent "github.com/Tangerg/scope/otel/agent"
)

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
	if err := start.SessionID.Validate(); err != nil {
		return runs.ExecutorRef{}, fmt.Errorf("execution: Interaction: %w", err)
	}
	if err := start.RunID.Validate(); err != nil {
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

func (i *InteractionExecutor) captureComposition(ctx context.Context, session *interactionSession, checkpoint *interactionCheckpointState) error {
	if checkpoint != nil {
		session.modelObservers = slices.Clone(checkpoint.modelObservers)
		session.modelContexts = slices.Clone(checkpoint.modelContexts)
		session.modelInputs = slices.Clone(checkpoint.modelInputs)
		session.toolProposals = slices.Clone(checkpoint.toolProposals)
		return nil
	}
	composition := i.config.CallComposition
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
	if err := i.bindComposition(ctx, session, checkpoint); err != nil {
		return nil, err
	}
	if len(session.modelObservers) > 0 {
		model, streamer = i.config.CallComposition.Bind(model, streamer, session.modelObservers)
	}
	observed, err := newObservedInteractionModel(model, streamer, session)
	if err != nil {
		return nil, fmt.Errorf("execution: observe Interaction model: %w", err)
	}
	storeTelemetry, err := otelagent.NewMiddleware(otelagent.MiddlewareConfig{})
	if err != nil {
		return nil, fmt.Errorf("execution: observe Interaction store: %w", err)
	}
	session.deltas = make(chan scoperun.Delta, i.policy.deltaBufferCapacity)
	session.engine, err = scoperun.New(session.lifetime.execution, scoperun.Config{
		Store: storeTelemetry.Store(session), Build: i.buildID.String(),
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
	if err := session.installDeployments(deployments); err != nil {
		return nil, errors.Join(err, deployments.close())
	}
	return session, nil
}

func (i *InteractionExecutor) bindComposition(ctx context.Context, session *interactionSession, checkpoint *interactionCheckpointState) error {
	if err := i.captureComposition(ctx, session, checkpoint); err != nil {
		return err
	}
	composition := i.config.CallComposition
	if len(session.toolProposals) > 0 {
		if err := composition.ValidateToolProposals(ctx, session.toolProposals); err != nil {
			return err
		}
	}
	activations, err := i.compositionActivations(ctx, session, checkpoint)
	if err != nil {
		return err
	}
	if err := session.bindCompositionActivations(activations); err != nil {
		return err
	}
	if len(session.modelInputs) > 0 {
		if err := composition.ValidateInput(ctx, session.modelInputs); err != nil {
			return err
		}
	}
	if len(session.modelContexts) > 0 {
		if err := composition.ValidateContext(ctx, session.modelContexts); err != nil {
			return err
		}
	}
	if len(session.modelObservers) > 0 {
		return composition.Validate(ctx, session.modelObservers)
	}
	return nil
}

func (i *InteractionExecutor) compositionActivations(ctx context.Context, session *interactionSession, checkpoint *interactionCheckpointState) ([]plugin.Activation, error) {
	if checkpoint != nil {
		return checkpoint.bindings, nil
	}
	return i.config.CallComposition.Activations(ctx, session.compositionBindings())
}

func (i *InteractionExecutor) interactionConfiguration(
	session *interactionSession,
	group domaintool.Group,
	toolAuthority string,
	depth uint32,
	instructions []corechat.Message,
) ([]byte, error) {
	configuration, err := scoperun.EncodePayload(struct {
		Provider               string                    `json:"provider"`
		Model                  string                    `json:"model"`
		Streaming              bool                      `json:"streaming"`
		MaxConcurrentToolCalls int                       `json:"maxConcurrentToolCalls"`
		Group                  domaintool.Group          `json:"group"`
		ToolAuthority          string                    `json:"toolAuthority,omitempty"`
		Depth                  uint32                    `json:"depth"`
		DelegateOptions        corechat.Options          `json:"delegateOptions"`
		Instructions           []corechat.Message        `json:"instructions,omitempty"`
		ModelObservers         []contributionBindingWire `json:"modelObservers"`
		CompositionActivations []string                  `json:"compositionActivations"`
		ModelContexts          []contributionBindingWire `json:"modelContexts"`
		ModelInputs            []contributionBindingWire `json:"modelInputs"`
		ToolProposals          []contributionBindingWire `json:"toolProposals"`
	}{
		Provider: session.start.ModelSelection.Provider(), Model: session.start.ModelSelection.Model(),
		Streaming:              i.config.StreamModelResponses,
		MaxConcurrentToolCalls: i.policy.maxConcurrentToolCalls,
		Group:                  group, ToolAuthority: toolAuthority, Depth: depth,
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
		VisibleTools      []toolConfigurationIdentity  `json:"visibleTools,omitempty"`
		DeferredTools     []toolConfigurationIdentity  `json:"deferredTools,omitempty"`
	}{installationDependencies(manifest.Installations), i.policy.toolResultOffload.identity(), visible, deferred})
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
		identities = append(identities, toolConfigurationIdentity{Definition: owner.binding.Contract().Definition(), Reference: ref.String(), SourceFingerprint: owner.sourceFingerprint.String(), Activation: activation.Identity().String()})
	}
	return identities, nil
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
