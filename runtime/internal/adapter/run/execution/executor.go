package execution

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"time"

	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
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

type InteractionChatResolver interface {
	ResolveChat(ctx context.Context, selection flamemodel.Selection) (modeladapter.ResolvedChat, error)
	RestoreChat(context.Context, flamemodel.Selection, *modeladapter.ProviderCapture) (modeladapter.ResolvedChat, error)
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
	OriginalToolResult(context.Context, domaintool.Ref, framevalue.Digest, framevalue.Digest, plugin.Activation, string, domaintool.Arguments) (*toolcontract.Output, error)
}

type InteractionExecutorConfig struct {
	Admission                 framehost.Admission
	ValidatePluginSources     func(context.Context, []plugin.Activation, []plugin.Dependency) error
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
	ToolResultOffloadBytes    *int
	UnknownEffectPollInterval *time.Duration
	StatePollInterval         *time.Duration
}

type InteractionExecutor struct {
	lifetime context.Context
	config   InteractionExecutorConfig
	policy   interactionExecutionPolicy
	buildID  runs.BuildID

	sessions interactionSessions
}

func NewInteractionExecutor(config InteractionExecutorConfig) (*InteractionExecutor, error) {
	if !config.Admission.Valid() {
		return nil, errors.New("execution: installation admission is required")
	}
	for _, capability := range []struct {
		name  string
		value any
	}{
		{name: "lifetime", value: config.Lifetime},
		{name: "plugin source validation", value: config.ValidatePluginSources},
		{name: "execution tree store", value: config.ExecutionTrees},
		{name: "model evidence", value: config.ModelEvidence},
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
		if lo.IsNil(capability.value) {
			return nil, fmt.Errorf("execution: Interaction %s is required", capability.name)
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
		buildID:  buildID,
		sessions: newInteractionSessions(),
	}, nil
}

func (i *InteractionExecutor) acceptsBuild(raw string) bool {
	identity, err := runs.ParseBuild(raw)
	return err == nil && identity == i.buildID
}

func interactionStartID(sessionID flameidentity.SessionID, runID flameidentity.RunID) scoperun.StartID {
	return scoperun.StartID{Scope: sessionID.String(), Key: runID.String()}
}

func newExecutorRef(sessionID flameidentity.SessionID) runs.ExecutorRef {
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

func (i *InteractionExecutor) BeginShutdown() {
	i.sessions.closeAdmission()
}

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

// The accepted Run uses the staged session lifecycle rather than the request
// context so a client disconnect cannot cancel durable execution.
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

func rootExecutionScope(start runs.RootExecutionStart) runs.ExecutionScope {
	return runs.ExecutionScope{
		SessionID: start.SessionID, CWD: start.CWD, WorkspaceCWD: start.WorkspaceCWD,
		Project: start.Project, Isolated: start.Isolated, GoalIncarnationID: start.GoalIncarnationID,
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

func (i *InteractionExecutor) Release(ctx context.Context, ref runs.ExecutorRef) error {
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

func (i *InteractionExecutor) RequestRootCancellation(
	ctx context.Context,
	ref runs.ExecutorRef,
	cancellation runs.CancellationID,
) error {
	if cancellation.IsZero() {
		return errors.New("execution: root cancellation identity is required")
	}
	session, err := i.session(ref)
	if err != nil {
		return err
	}
	process := session.state.processHandle()
	if process == nil {
		return runs.ErrExecutorNotLive
	}
	if err := process.Cancel(ctx, cancellationControlPrefix+cancellation.String()); err != nil &&
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
	selection flamemodel.Selection,
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

func executionOptions(selection flamemodel.Selection, options *corechat.Options) corechat.Options {
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

func delegatedExecutionOptions(start runs.RootExecutionStart) corechat.Options {
	options := executionOptions(start.ModelSelection, start.Options)
	options.OutputFormat = nil
	options.Stop = nil
	return options
}
