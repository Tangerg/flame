package execution

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

var _ interaction.AdvertisingTool = (*toolset.Discovery)(nil)

type interactionDeploymentSet struct {
	// Connection publication can occur between group resolutions. Recovery must
	// retain each group's authority rather than reconstructing their union.
	manifests         map[domaintool.Group]toolset.Manifest
	root              agent.Deployment
	delegatesByParent map[agent.DeploymentRef]map[string]agent.Deployment
	managedChildren   map[agent.DeploymentRef]struct{}
	toolChildren      map[agent.DeploymentRef]struct{}
	// Immutable Scope Contracts are indexed by the exact Tool deployment so a
	// checkpoint cannot borrow another layer's input schema.
	toolContracts map[agent.DeploymentRef]map[domaintool.Ref]toolcontract.Contract
	toolSets      map[agent.DeploymentRef]interaction.ToolSet
	observedTools map[agent.DeploymentRef]map[domaintool.Ref]*observedInteractionTool
	treeLimits    agent.TreeLimits
}

func (i *interactionDeploymentSet) close() error {
	if i == nil {
		return nil
	}
	var err error
	for _, manifest := range i.manifests {
		err = errors.Join(err, manifest.Close())
	}
	return err
}

func (i *interactionDeploymentSet) managedChild(reference agent.DeploymentRef) bool {
	_, found := i.managedChildren[reference]
	return found
}

// toolChild reports whether a child Process is one ordinary Tool call. A Tool
// call is an Item of the Run that made it, not a Run of its own, so it carries
// no Delegate binding and projects no child Run.
func (i *interactionDeploymentSet) toolChild(reference agent.DeploymentRef) bool {
	_, found := i.toolChildren[reference]
	return found
}

func (i *interactionDeploymentSet) hasToolReceipts() bool {
	if i == nil {
		return false
	}
	for _, tools := range i.observedTools {
		for _, tool := range tools {
			if tool.receiptEvidence != nil {
				return true
			}
		}
	}
	return false
}

func (i *interactionDeploymentSet) delegateTarget(
	parent agent.DeploymentRef,
	name string,
) (agent.Deployment, bool) {
	target, found := i.delegatesByParent[parent][name]
	return target, found
}

func (i *InteractionExecutor) buildInteractionDeployments(
	ctx context.Context,
	session *interactionSession,
	start runs.RootExecutionStart,
	model *observedInteractionModel,
	counter modelContextInputTokenCounter,
	checkpoint *interactionCheckpointState,
) (*interactionDeploymentSet, error) {
	builder, err := i.newInteractionDeploymentBuilder(
		ctx, session, start, model, counter, checkpoint,
	)
	if err != nil {
		return nil, err
	}
	deployments, err := builder.build(ctx)
	if err != nil {
		return nil, errors.Join(err, builder.deployments.close())
	}
	return deployments, nil
}

type interactionDeploymentBuilder struct {
	executor          *InteractionExecutor
	session           *interactionSession
	start             runs.RootExecutionStart
	model             *observedInteractionModel
	counter           modelContextInputTokenCounter
	maxDepth          uint32
	instructions      []corechat.Message
	rootManifest      toolset.Manifest
	delegatedManifest toolset.Manifest
	deployments       *interactionDeploymentSet
}

func (i *InteractionExecutor) newInteractionDeploymentBuilder(
	ctx context.Context,
	session *interactionSession,
	start runs.RootExecutionStart,
	model *observedInteractionModel,
	counter modelContextInputTokenCounter,
	checkpoint *interactionCheckpointState,
) (_ *interactionDeploymentBuilder, err error) {
	instructions, err := interactionInstructionContext(start.WorkingContext)
	if err != nil {
		return nil, err
	}
	rootManifest, err := i.resolveInteractionManifest(ctx, domaintool.GroupRoot, checkpoint)
	if err != nil {
		return nil, err
	}
	builder := &interactionDeploymentBuilder{
		executor: i, session: session, start: start, model: model, counter: counter,
		instructions: instructions, rootManifest: rootManifest,
		deployments: &interactionDeploymentSet{
			manifests:         map[domaintool.Group]toolset.Manifest{domaintool.GroupRoot: rootManifest},
			toolChildren:      make(map[agent.DeploymentRef]struct{}),
			toolContracts:     make(map[agent.DeploymentRef]map[domaintool.Ref]toolcontract.Contract),
			toolSets:          make(map[agent.DeploymentRef]interaction.ToolSet),
			observedTools:     make(map[agent.DeploymentRef]map[domaintool.Ref]*observedInteractionTool),
			delegatesByParent: make(map[agent.DeploymentRef]map[string]agent.Deployment),
			managedChildren:   make(map[agent.DeploymentRef]struct{}),
			treeLimits:        agent.TreeLimits{MaxDepth: 2, MaxActiveChildren: uint32(i.policy.maxConcurrentToolCalls)},
		},
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, builder.deployments.close())
		}
	}()
	if start.ChildRunAdmissionEnabled {
		builder.maxDepth = defaultDelegateDepth
		builder.deployments.treeLimits.MaxDepth = defaultDelegateDepth + 1
		builder.deployments.treeLimits.MaxActiveChildren = uint32(min(uint64(i.policy.maxConcurrentToolCalls)+runs.MaxActiveChildRuns, math.MaxUint32))
	}
	if builder.maxDepth > 0 {
		builder.delegatedManifest, err = i.resolveInteractionManifest(ctx, domaintool.GroupDelegated, checkpoint)
		if err != nil {
			return nil, err
		}
		builder.deployments.manifests[domaintool.GroupDelegated] = builder.delegatedManifest
	}
	return builder, nil
}

func (i *interactionDeploymentBuilder) build(ctx context.Context) (*interactionDeploymentSet, error) {
	var next agent.Deployment
	for depth := int(i.maxDepth); depth >= 0; depth-- {
		deployment, err := i.buildAtDepth(ctx, depth, next)
		if err != nil {
			return nil, err
		}
		if depth > 0 {
			i.deployments.managedChildren[deployment.DeploymentRef()] = struct{}{}
		}
		if next.Valid() {
			i.deployments.delegatesByParent[deployment.DeploymentRef()] = map[string]agent.Deployment{
				string(builtincontract.DelegateTask): next,
			}
		}
		next = deployment
	}
	i.deployments.root = next
	return i.deployments, nil
}

func (i *interactionDeploymentBuilder) buildAtDepth(ctx context.Context, depth int, next agent.Deployment) (agent.Deployment, error) {
	group, manifest, definitionName, definitionDescription := i.layerIdentity(depth)
	delegates, err := i.delegateLayer(depth, next)
	if err != nil {
		return agent.Deployment{}, err
	}
	visible, deferred, contracts, err := wrapInteractionTools(
		ctx,
		manifest,
		i.session,
		i.executor.config,
		i.executor.policy.toolResultOffload,
		i.start,
	)
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("execution: wrap Interaction tools at depth %d: %w", depth, err)
	}
	manifest.Visible, manifest.Deferred = visible, deferred
	// An ordinary Tool is a child Process with its own Deployment now, so the
	// Tool authority belongs to the Definition rather than to the model
	// boundary. Flame grants no Framework capability names, so a Tool child runs
	// under the empty set its parent also holds. A layer with no Tools carries
	// the zero ToolSet, which is how the absence is spelled.
	var tools interaction.ToolSet
	if len(visible)+len(deferred) > 0 {
		configuration, err := i.executor.interactionToolConfiguration(manifest)
		if err != nil {
			return agent.Deployment{}, err
		}
		tools, err = interaction.NewToolSet(interaction.ToolSetConfig{
			Name: definitionName + ".tools", Description: definitionDescription,
			Tools: visible, DeferredTools: deferred,
			DecorateDispatcher: func(inner agent.Dispatcher) (agent.Dispatcher, error) {
				return &interactionDispatcher{inner: inner, session: i.session}, nil
			},
			ImplementationDigest: agent.ComputeDigest([]byte(i.executor.buildID.String())),
			ConfigurationDigest:  agent.ComputeDigest(configuration),
		})
		if err != nil {
			return agent.Deployment{}, fmt.Errorf("execution: build Interaction Tool set at depth %d: %w", depth, err)
		}
	}
	definitionConfig := interaction.DefinitionConfig{
		Name: definitionName, Description: definitionDescription,
		Delegates: delegates,
	}
	// Tool scheduling policy belongs to a layer that has Tools. A layer without
	// them carries neither, so the absence is one fact rather than two.
	if tools.Configured() {
		definitionConfig.Tools = tools
		definitionConfig.MaxConcurrentToolCalls = i.executor.policy.maxConcurrentToolCalls
		toolDeployment := tools.Deployment()
		i.deployments.managedChildren[toolDeployment.DeploymentRef()] = struct{}{}
		i.deployments.toolChildren[toolDeployment.DeploymentRef()] = struct{}{}
		i.deployments.toolContracts[toolDeployment.DeploymentRef()] = contracts
		i.deployments.toolSets[toolDeployment.DeploymentRef()] = tools
		observed := make(map[domaintool.Ref]*observedInteractionTool)
		for _, executable := range append(visible, deferred...) {
			owner, found, err := toolcontract.Capability[*observedInteractionTool](executable)
			if err != nil || !found {
				return agent.Deployment{}, errors.Join(errors.New("execution: Tool owner is unavailable"), err)
			}
			observed[owner.ref] = owner
		}
		i.deployments.observedTools[toolDeployment.DeploymentRef()] = observed
	}
	definition, err := interaction.NewDefinition(definitionConfig)
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("execution: build Interaction definition at depth %d: %w", depth, err)
	}
	contextReducer := newInteractionModelContextReducer(
		i.executor.config.ModelContextCompactor,
		i.executor.config.ModelContextState,
		i.session,
		i.start,
		i.instructions,
		deferredCatalogMessage(manifest),
		i.counter,
	)
	// The Dispatcher takes exactly one model capability. Streaming is requested
	// by configuration but offered only by a provider that has it, so a
	// non-streaming provider answers complete responses without a second switch.
	dispatcherConfig := interaction.DispatcherConfig{ModelContextReducer: contextReducer}
	model := i.model
	if i.executor.config.StreamModelResponses && model.Streams() {
		dispatcherConfig.Streamer = model
	} else {
		dispatcherConfig.Model = model
	}
	dispatcher, err := interaction.NewDispatcher(definition, dispatcherConfig)
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("execution: build Interaction dispatcher at depth %d: %w", depth, err)
	}
	deploymentDefinition, err := i.deploymentDefinition(depth, definition)
	if err != nil {
		return agent.Deployment{}, err
	}
	configuration, err := i.executor.interactionConfiguration(
		i.session, group, uint32(depth),
		i.instructions,
	)
	if err != nil {
		return agent.Deployment{}, err
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           deploymentDefinition,
		Dispatcher:           &interactionDispatcher{inner: dispatcher, session: i.session},
		ImplementationDigest: agent.ComputeDigest([]byte(i.executor.buildID.String())),
		ConfigurationDigest:  agent.ComputeDigest(configuration),
	})
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("execution: build Interaction deployment at depth %d: %w", depth, err)
	}
	return deployment, nil
}

// deferredCatalogMessage projects a layer's frozen deferred Tool catalog as
// the last message of each of its model requests. It is a User message because
// providers hoist System messages into the instruction block, which precedes
// the conversation in the cache prefix; the Runtime context framing marks it
// as Runtime context rather than user input.
func deferredCatalogMessage(manifest toolset.Manifest) []corechat.Message {
	catalog := manifest.DeferredCatalog()
	if catalog == "" {
		return nil
	}
	return []corechat.Message{corechat.NewUserMessage(corechat.NewTextPart(
		FrameRuntimeContext(RuntimeContextDeferredTools, catalog),
	))}
}

func (i *interactionDeploymentBuilder) layerIdentity(
	depth int,
) (domaintool.Group, toolset.Manifest, string, string) {
	if depth == 0 {
		return domaintool.GroupRoot, i.rootManifest, interactionDefinitionName, interactionDefinitionDescription
	}
	return domaintool.GroupDelegated,
		i.delegatedManifest,
		"flame.runtime.interaction.delegate.depth" + strconv.Itoa(depth),
		"Run one isolated delegated Flame interaction."
}

func (i *interactionDeploymentBuilder) delegateLayer(
	depth int,
	next agent.Deployment,
) ([]interaction.Delegate, error) {
	if !next.Valid() {
		return nil, nil
	}
	delegate, err := interaction.NewDelegate(interaction.DelegateConfig{
		Name: string(builtincontract.DelegateTask), Description: delegateDescription,
		Deployment: next,
	})
	if err != nil {
		return nil, fmt.Errorf("execution: build Delegate at depth %d: %w", depth, err)
	}
	return []interaction.Delegate{delegate}, nil
}

func (i *interactionDeploymentBuilder) deploymentDefinition(
	depth int,
	definition *interaction.Definition,
) (agent.Definition, error) {
	if depth == 0 {
		return definition, nil
	}
	return newDelegatedInteractionDefinition(
		"flame.runtime.delegated_task.depth"+strconv.Itoa(depth), definition, i.instructions, delegatedExecutionOptions(i.session.start),
	)
}

func (i *InteractionExecutor) resolveInteractionManifest(
	ctx context.Context,
	group domaintool.Group,
	checkpoint *interactionCheckpointState,
) (toolset.Manifest, error) {
	var retained *toolset.Retained
	if checkpoint != nil {
		captured, found := checkpoint.mcpManifests[group]
		if !found {
			return toolset.Manifest{}, errors.New("execution: captured Tool group is missing")
		}
		retained = &toolset.Retained{MCP: captured, Bindings: checkpoint.bindings, Resources: checkpoint.resources}
	}
	if i.config.ToolResolver == nil {
		return toolset.Manifest{}, nil
	}
	manifest, err := i.config.ToolResolver.Manifest(ctx, group, retained)
	if err != nil {
		return toolset.Manifest{}, fmt.Errorf("execution: resolve Interaction %s Tools: %w", group, err)
	}
	return manifest.Clone(), nil
}

func interactionInstructionContext(messages []corechat.Message) ([]corechat.Message, error) {
	var instructions []corechat.Message
	for index := range messages {
		if messages[index].Role != corechat.RoleSystem {
			break
		}
		if err := messages[index].Validate(); err != nil {
			return nil, fmt.Errorf("execution: invalid Interaction instruction[%d]: %w", index, err)
		}
		provenance, found, decodeErr := messages[index].Metadata.Decode[contextSources](
			contextProvenanceMetadataKey,
		)
		if decodeErr != nil {
			return nil, fmt.Errorf("execution: decode Interaction instruction[%d] provenance: %w", index, decodeErr)
		}
		if !found {
			break
		}
		if err := provenance.validate(); err != nil {
			return nil, fmt.Errorf("execution: Interaction instruction[%d] provenance: %w", index, err)
		}
		instructions = append(instructions, messages[index].Clone())
	}
	return instructions, nil
}
