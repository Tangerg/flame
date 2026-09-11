package agentexec

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

type interactionDeploymentSet struct {
	root              agent.Deployment
	byRef             map[agent.DeploymentRef]agent.Deployment
	delegatesByParent map[agent.DeploymentRef]map[string]agent.DeploymentRef
	managedChildren   map[agent.DeploymentRef]struct{}
	toolChildren      map[agent.DeploymentRef]struct{}
	treeLimits        agent.TreeLimits
}

func (i *interactionDeploymentSet) Resolve(
	reference agent.DeploymentRef,
) (agent.Deployment, error) {
	if i == nil {
		return agent.Deployment{}, agent.ErrInvalidDeploymentRef
	}
	deployment, found := i.byRef[reference]
	if !found {
		return agent.Deployment{}, agent.ErrInvalidDeploymentRef
	}
	return deployment, nil
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

func (i *interactionDeploymentSet) delegateTarget(
	parent agent.DeploymentRef,
	name string,
) (agent.DeploymentRef, bool) {
	target, found := i.delegatesByParent[parent][name]
	return target, found
}

func (i *InteractionExecutor) buildInteractionDeployments(
	ctx context.Context,
	session *interactionSession,
	start runs.RootExecutionStart,
	model *observedInteractionModel,
	counter ModelContextInputTokenCounter,
	maxModelCalls uint32,
) (*interactionDeploymentSet, error) {
	builder, err := i.newInteractionDeploymentBuilder(
		ctx, session, start, model, counter, maxModelCalls,
	)
	if err != nil {
		return nil, err
	}
	return builder.build()
}

type interactionDeploymentBuilder struct {
	executor          *InteractionExecutor
	session           *interactionSession
	start             runs.RootExecutionStart
	model             *observedInteractionModel
	counter           ModelContextInputTokenCounter
	maxModelCalls     uint32
	delegation        effectiveInteractionDelegation
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
	counter ModelContextInputTokenCounter,
	maxModelCalls uint32,
) (*interactionDeploymentBuilder, error) {
	instructions, err := interactionInstructionContext(start.WorkingContext)
	if err != nil {
		return nil, err
	}
	rootManifest, err := i.resolveInteractionManifest(ctx, domaintool.GroupRoot)
	if err != nil {
		return nil, err
	}
	builder := &interactionDeploymentBuilder{
		executor: i, session: session, start: start, model: model, counter: counter,
		maxModelCalls: maxModelCalls, delegation: i.policy.delegation,
		instructions: instructions, rootManifest: rootManifest,
		deployments: &interactionDeploymentSet{
			byRef:             make(map[agent.DeploymentRef]agent.Deployment),
			toolChildren:      make(map[agent.DeploymentRef]struct{}),
			delegatesByParent: make(map[agent.DeploymentRef]map[string]agent.DeploymentRef),
			managedChildren:   make(map[agent.DeploymentRef]struct{}),
			treeLimits:        toolTreeLimits(maxModelCalls, i.policy),
		},
	}
	if start.ChildRunAdmissionEnabled {
		builder.maxDepth = builder.delegation.treeLimits.MaxDepth
		builder.deployments.treeLimits = withToolTreeCapacity(
			builder.delegation.treeLimits, maxModelCalls, i.policy,
		)
	}
	if builder.maxDepth > 0 {
		builder.delegatedManifest, err = i.resolveInteractionManifest(ctx, domaintool.GroupDelegated)
		if err != nil {
			return nil, err
		}
	}
	return builder, nil
}

func (i *interactionDeploymentBuilder) build() (*interactionDeploymentSet, error) {
	var next agent.Deployment
	for depth := int(i.maxDepth); depth >= 0; depth-- {
		deployment, err := i.buildAtDepth(depth, next)
		if err != nil {
			return nil, err
		}
		i.deployments.byRef[deployment.DeploymentRef()] = deployment
		if depth > 0 {
			i.deployments.managedChildren[deployment.DeploymentRef()] = struct{}{}
		}
		if next.Valid() {
			i.deployments.delegatesByParent[deployment.DeploymentRef()] = map[string]agent.DeploymentRef{
				domaintool.DelegateTask: next.DeploymentRef(),
			}
		}
		next = deployment
	}
	i.deployments.root = next
	return i.deployments, nil
}

func (i *interactionDeploymentBuilder) buildAtDepth(depth int, next agent.Deployment) (agent.Deployment, error) {
	group, manifest, definitionName, definitionDescription := i.layerIdentity(depth)
	delegates, delegateBudget, err := i.delegateLayer(depth, next)
	if err != nil {
		return agent.Deployment{}, err
	}
	visible, deferred, err := wrapInteractionTools(
		manifest,
		i.session,
		i.executor.config,
		i.executor.policy.toolResultOffload,
		i.start,
	)
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("agentexec: wrap Interaction tools at depth %d: %w", depth, err)
	}
	// An ordinary Tool is a child Process with its own Deployment now, so the
	// Tool authority belongs to the Definition rather than to the model
	// boundary. Flame grants no Framework capability names, so a Tool child runs
	// under the empty set its parent also holds. A layer with no Tools carries
	// the zero ToolSet, which is how the absence is spelled.
	var tools interaction.ToolSet
	if len(visible)+len(deferred) > 0 {
		tools, err = interaction.NewToolSet(interaction.ToolSetConfig{
			Name: definitionName + ".tools", Description: definitionDescription,
			Tools: visible, DeferredTools: deferred,
			ImplementationDigest: agent.ComputeDigest([]byte(i.executor.implementationIdentity.String())),
			ConfigurationDigest:  agent.ComputeDigest([]byte(i.executor.configurationIdentity.String())),
		})
		if err != nil {
			return agent.Deployment{}, fmt.Errorf("agentexec: build Interaction Tool set at depth %d: %w", depth, err)
		}
	}
	definitionConfig := interaction.DefinitionConfig{
		Name: definitionName, Description: definitionDescription,
		MaxModelCalls: i.maxModelCalls,
		Delegates:     delegates,
	}
	// Tool scheduling policy belongs to a layer that has Tools. A layer without
	// them carries neither, so the absence is one fact rather than two.
	if tools.Valid() {
		definitionConfig.Tools = tools
		definitionConfig.ToolBudget = i.executor.policy.toolBudget
		definitionConfig.MaxConcurrentToolCalls = i.executor.policy.maxConcurrentToolCalls
		// A Tool call is a child Process, so the Engine has to be able to resolve
		// the Deployment it runs in through this same resolver.
		toolDeployment := tools.Deployment()
		i.deployments.byRef[toolDeployment.DeploymentRef()] = toolDeployment
		i.deployments.managedChildren[toolDeployment.DeploymentRef()] = struct{}{}
		i.deployments.toolChildren[toolDeployment.DeploymentRef()] = struct{}{}
	}
	definition, err := interaction.NewDefinition(definitionConfig)
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("agentexec: build Interaction definition at depth %d: %w", depth, err)
	}
	contextReducer := newInteractionModelContextReducer(
		i.executor.config.ModelContextCompactor,
		i.executor.config.ModelContextState,
		i.session,
		i.start,
		i.instructions,
		i.counter,
	)
	// The Dispatcher takes exactly one model capability. Streaming is requested
	// by configuration but offered only by a provider that has it, so a
	// non-streaming provider answers complete responses without a second switch.
	dispatcherConfig := interaction.DispatcherConfig{ModelContextReducer: contextReducer}
	if i.executor.config.StreamModelResponses && i.model.Streams() {
		dispatcherConfig.Streamer = i.model
	} else {
		dispatcherConfig.Model = i.model
	}
	dispatcher, err := interaction.NewDispatcher(definition, dispatcherConfig)
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("agentexec: build Interaction dispatcher at depth %d: %w", depth, err)
	}
	deploymentDefinition, err := i.deploymentDefinition(depth, definition)
	if err != nil {
		return agent.Deployment{}, err
	}
	var delegateRef agent.DeploymentRef
	if next.Valid() {
		delegateRef = next.DeploymentRef()
	}
	configuration, err := i.executor.interactionConfiguration(
		i.session, i.maxModelCalls, manifest, group, uint32(depth), delegateRef,
		delegateBudget, i.instructions,
	)
	if err != nil {
		return agent.Deployment{}, err
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           deploymentDefinition,
		Dispatcher:           &interactionDispatcher{inner: dispatcher, session: i.session},
		ImplementationDigest: agent.ComputeDigest([]byte(i.executor.implementationIdentity.String())),
		ConfigurationDigest:  agent.ComputeDigest(configuration),
	})
	if err != nil {
		return agent.Deployment{}, fmt.Errorf("agentexec: build Interaction deployment at depth %d: %w", depth, err)
	}
	return deployment, nil
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
) ([]interaction.Delegate, agent.Budget, error) {
	if !next.Valid() {
		return nil, agent.Budget{}, nil
	}
	budget, err := delegateSubtreeBudget(i.delegation.processBudget, i.maxDepth-uint32(depth))
	if err != nil {
		return nil, agent.Budget{}, fmt.Errorf("agentexec: allocate Delegate at depth %d: %w", depth, err)
	}
	delegate, err := interaction.NewDelegate(interaction.DelegateConfig{
		Name: domaintool.DelegateTask, Description: delegateDescription,
		Deployment: next, Budget: budget,
	})
	if err != nil {
		return nil, agent.Budget{}, fmt.Errorf("agentexec: build Delegate at depth %d: %w", depth, err)
	}
	return []interaction.Delegate{delegate}, budget, nil
}

func (i *interactionDeploymentBuilder) deploymentDefinition(
	depth int,
	definition *interaction.Definition,
) (agent.Definition, error) {
	if depth == 0 {
		return definition, nil
	}
	return newDelegatedInteractionDefinition(
		"flame.runtime.delegated_task.depth"+strconv.Itoa(depth), definition, i.instructions,
	)
}

func (i *InteractionExecutor) resolveInteractionManifest(
	ctx context.Context,
	group domaintool.Group,
) (toolset.Manifest, error) {
	if i.config.ToolResolver == nil {
		return toolset.Manifest{}, nil
	}
	manifest, err := i.config.ToolResolver.Manifest(ctx, group)
	if err != nil {
		return toolset.Manifest{}, fmt.Errorf("agentexec: resolve Interaction %s Tools: %w", group, err)
	}
	manifest = manifest.Clone()
	if err := validateToolManifest(manifest); err != nil {
		return toolset.Manifest{}, err
	}
	if err := i.validateInteractionTools(manifest); err != nil {
		return toolset.Manifest{}, err
	}
	return manifest, nil
}

func interactionInstructionContext(messages []corechat.Message) ([]corechat.Message, error) {
	var instructions []corechat.Message
	for index := range messages {
		if messages[index].Role != corechat.RoleSystem {
			break
		}
		if err := messages[index].Validate(); err != nil {
			return nil, fmt.Errorf("agentexec: invalid Interaction instruction[%d]: %w", index, err)
		}
		provenance, found, decodeErr := messages[index].Metadata.Decode[contextSources](
			contextProvenanceMetadataKey,
		)
		if decodeErr != nil {
			return nil, fmt.Errorf("agentexec: decode Interaction instruction[%d] provenance: %w", index, decodeErr)
		}
		if !found {
			break
		}
		_, sessionState, err := provenance.replaceableSessionState()
		if err != nil {
			return nil, fmt.Errorf("agentexec: Interaction instruction[%d] provenance: %w", index, err)
		}
		if sessionState {
			break
		}
		instructions = append(instructions, messages[index].Clone())
	}
	return instructions, nil
}

// toolTreeLimits sizes a tree whose only children are ordinary Tool calls. Every
// Tool call is one child Process, and every Tool call originates in a model
// response, so the Interaction's own model-call ceiling bounds how many a Run
// can make: maxModelCalls responses, each requesting at most one batch.
// MaxChildren and MaxTreeProcesses are lifetime counts, so they are the first
// ceiling Flame has ever placed on Tool calls per Run; ToolBatchCeiling raises
// it without touching this derivation.
func toolTreeLimits(maxModelCalls uint32, policy interactionExecutionPolicy) agent.TreeLimits {
	calls := toolCallCeiling(maxModelCalls, policy)
	return agent.TreeLimits{
		MaxDepth: 2, MaxChildren: calls,
		MaxActiveChildren: uint32(policy.maxConcurrentToolCalls),
		MaxTreeProcesses:  calls + 1,
	}
}

// withToolTreeCapacity widens delegation limits so each Interaction layer can
// still call its Tools. A delegated layer is a Process that also spawns one
// child per Tool call, so the delegate ceilings alone would starve them.
func withToolTreeCapacity(
	limits agent.TreeLimits,
	maxModelCalls uint32,
	policy interactionExecutionPolicy,
) agent.TreeLimits {
	calls := toolCallCeiling(maxModelCalls, policy)
	limits.MaxDepth++
	limits.MaxChildren = saturatingAdd(limits.MaxChildren, calls)
	limits.MaxActiveChildren = saturatingAdd(limits.MaxActiveChildren, uint32(policy.maxConcurrentToolCalls))
	limits.MaxTreeProcesses = saturatingAdd(
		limits.MaxTreeProcesses, saturatingMultiply(limits.MaxTreeProcesses, calls),
	)
	return limits
}

func toolCallCeiling(maxModelCalls uint32, policy interactionExecutionPolicy) uint32 {
	return saturatingMultiply(maxModelCalls, policy.toolBatchCeiling)
}

func saturatingAdd(left, right uint32) uint32 {
	if left > math.MaxUint32-right {
		return math.MaxUint32
	}
	return left + right
}

func saturatingMultiply(left, right uint32) uint32 {
	if left != 0 && right > math.MaxUint32/left {
		return math.MaxUint32
	}
	return left * right
}
