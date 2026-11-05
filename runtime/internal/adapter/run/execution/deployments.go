package execution

import (
	"context"
	"errors"
	"fmt"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

var _ interaction.AdvertisingTool = (*toolset.Discovery)(nil)

type interactionDeploymentSet struct {
	// Connection publication can occur between group resolutions. Recovery must
	// retain each group's authority rather than reconstructing their union.
	manifests           map[domaintool.Group]toolset.Manifest
	root                scoperun.Program[scoperun.Payload, scoperun.Payload]
	delegateSource      domaintool.Ref
	delegatesByParent   map[scoperun.ProgramRef]map[string]scoperun.Program[scoperun.Payload, scoperun.Payload]
	managedChildren     map[scoperun.ProgramRef]struct{}
	interactionChildren map[scoperun.ProgramRef]struct{}
	interactions        map[scoperun.ProgramRef]scoperun.Program[interaction.Input, interaction.Output]
	// Immutable Scope Contracts are indexed by the exact Tool deployment so a
	// checkpoint cannot borrow another layer's input schema.
	toolContracts map[scoperun.ProgramRef]map[domaintool.Ref]toolcontract.Contract
	toolSets      map[scoperun.ProgramRef]interaction.ToolSet
	observedTools map[scoperun.ProgramRef]map[domaintool.Ref]*observedInteractionTool
	treeLimits    scoperun.TreeLimits
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

func (i *interactionDeploymentSet) managedChild(reference scoperun.ProgramRef) bool {
	_, found := i.managedChildren[reference]
	return found
}

// interactionChild is the inner model loop of a delegated task. The workflow
// adapter owns the product child Run; its Interaction is execution machinery.
func (i *interactionDeploymentSet) interactionChild(reference scoperun.ProgramRef) bool {
	_, found := i.interactionChildren[reference]
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
	parent scoperun.ProgramRef,
	name string,
) (scoperun.Program[scoperun.Payload, scoperun.Payload], bool) {
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
	delegateSource, err := domaintool.BuiltIn(builtincontract.DelegateTask)
	if err != nil {
		return nil, err
	}
	builder := &interactionDeploymentBuilder{
		executor: i, session: session, start: start, model: model, counter: counter,
		instructions: instructions, rootManifest: rootManifest,
		deployments: &interactionDeploymentSet{
			delegateSource:      delegateSource,
			manifests:           map[domaintool.Group]toolset.Manifest{domaintool.GroupRoot: rootManifest},
			interactionChildren: make(map[scoperun.ProgramRef]struct{}),
			interactions:        make(map[scoperun.ProgramRef]scoperun.Program[interaction.Input, interaction.Output]),
			toolContracts:       make(map[scoperun.ProgramRef]map[domaintool.Ref]toolcontract.Contract),
			toolSets:            make(map[scoperun.ProgramRef]interaction.ToolSet),
			observedTools:       make(map[scoperun.ProgramRef]map[domaintool.Ref]*observedInteractionTool),
			delegatesByParent:   make(map[scoperun.ProgramRef]map[string]scoperun.Program[scoperun.Payload, scoperun.Payload]),
			managedChildren:     make(map[scoperun.ProgramRef]struct{}),
			treeLimits:          interactionTreeLimits(start),
		},
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, builder.deployments.close())
		}
	}()
	if start.ChildRunAdmissionEnabled {
		builder.maxDepth = defaultDelegateDepth
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
	var next scoperun.Program[scoperun.Payload, scoperun.Payload]
	for depth := int(i.maxDepth); depth >= 0; depth-- {
		deployment, err := i.buildAtDepth(ctx, depth, next)
		if err != nil {
			return nil, err
		}
		if depth > 0 {
			i.deployments.managedChildren[deployment.ProgramRef()] = struct{}{}
		}
		next = deployment
	}
	i.deployments.root = next
	return i.deployments, nil
}

func (i *interactionDeploymentBuilder) buildAtDepth(ctx context.Context, depth int, next scoperun.Program[scoperun.Payload, scoperun.Payload]) (scoperun.Program[scoperun.Payload, scoperun.Payload], error) {
	group, manifest, definitionName, definitionDescription := i.layerIdentity(depth)
	delegates, err := i.delegateLayer(depth, next)
	if err != nil {
		return scoperun.Program[scoperun.Payload, scoperun.Payload]{}, err
	}
	visible, deferred, contracts, observed, err := wrapInteractionTools(
		ctx,
		manifest,
		i.session,
		i.executor.config,
		i.executor.policy.toolResultOffload,
		i.start,
	)
	if err != nil {
		return scoperun.Program[scoperun.Payload, scoperun.Payload]{}, fmt.Errorf("execution: wrap Interaction tools at depth %d: %w", depth, err)
	}
	var tools interaction.ToolSet
	if len(visible)+len(deferred) > 0 {
		configuration, err := i.executor.interactionToolConfiguration(manifest)
		if err != nil {
			return scoperun.Program[scoperun.Payload, scoperun.Payload]{}, err
		}
		tools, err = interaction.NewToolSet(interaction.ToolSetConfig{
			Revision: i.executor.buildID.String() + ":" + scoperun.ComputeDigest(configuration).String(),
			Tools:    visible, DeferredTools: deferred,
		})
		if err != nil {
			return scoperun.Program[scoperun.Payload, scoperun.Payload]{}, fmt.Errorf("execution: bind Interaction Tools at depth %d: %w", depth, err)
		}
	}
	configuration, err := i.executor.interactionConfiguration(i.session, group, uint32(depth), i.instructions)
	if err != nil {
		return scoperun.Program[scoperun.Payload, scoperun.Payload]{}, err
	}
	contextProcessor := newInteractionModelContextReducer(
		i.executor.config.ModelContextCompactor, i.executor.config.ModelContextState,
		i.session, i.start, i.instructions, deferredCatalogMessage(manifest), i.counter,
	)
	model := interaction.CallModel(i.model)
	if i.executor.config.StreamModelResponses && i.model.Streams() {
		model = interaction.StreamModel(interaction.StreamModelConfig{Streamer: i.model})
	}
	config := interaction.Config{
		Name: definitionName, Description: definitionDescription,
		Revision: i.executor.buildID.String() + ":" + scoperun.ComputeDigest(configuration).String(),
		Model:    model, ContextProcessor: contextProcessor, Tools: tools, Delegates: delegates,
	}
	if tools.Configured() {
		config.MaxConcurrentToolCalls = i.executor.policy.maxConcurrentToolCalls
	}
	program, err := interaction.Compile(i.session.engine.Compiler(), config)
	if err != nil {
		return scoperun.Program[scoperun.Payload, scoperun.Payload]{}, fmt.Errorf("execution: compile Interaction at depth %d: %w", depth, err)
	}
	reference := program.ProgramRef()
	i.deployments.interactions[reference] = program
	i.deployments.toolContracts[reference] = contracts
	i.deployments.toolSets[reference] = tools
	i.deployments.observedTools[reference] = observed
	if next.Valid() {
		i.deployments.delegatesByParent[reference] = map[string]scoperun.Program[scoperun.Payload, scoperun.Payload]{
			i.deployments.delegateSource.ModelName(): next,
		}
	}
	deployment := program.Erase()
	if depth > 0 {
		i.deployments.managedChildren[reference] = struct{}{}
		i.deployments.interactionChildren[reference] = struct{}{}
		adapted, err := compileDelegatedInteraction(i.session.engine.Compiler(),
			fmt.Sprintf("flame.runtime.delegated_task.depth%d", depth),
			config.Revision, program, i.instructions, delegatedExecutionOptions(i.session.start))
		if err != nil {
			return scoperun.Program[scoperun.Payload, scoperun.Payload]{}, err
		}
		deployment = adapted.Erase()
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
	return domaintool.GroupDelegated, i.delegatedManifest, delegatedDefinitionName(depth), delegatedDefinitionDescription
}

// delegatedDefinitionName names the interaction at one delegation depth; each
// depth is a distinct agent definition under the root interaction's name.
func delegatedDefinitionName(depth int) string {
	return fmt.Sprintf("%s.delegate.depth%d", interactionDefinitionName, depth)
}

func (i *interactionDeploymentBuilder) delegateLayer(
	depth int,
	next scoperun.Program[scoperun.Payload, scoperun.Payload],
) ([]interaction.Delegate, error) {
	if !next.Valid() {
		return nil, nil
	}
	delegate, err := interaction.NewDelegate(interaction.DelegateConfig{
		Name: i.deployments.delegateSource.ModelName(), Description: delegateDescription,
		Child: scoperun.ChildBinding{Program: next},
	})
	if err != nil {
		return nil, fmt.Errorf("execution: build Delegate at depth %d: %w", depth, err)
	}
	return []interaction.Delegate{delegate}, nil
}

func interactionTreeLimits(start runs.RootExecutionStart) scoperun.TreeLimits {
	limits := scoperun.DefaultTreeLimits()
	// Each delegated task is a workflow containing one Interaction child.
	limits.MaxDepth = 2 * defaultDelegateDepth
	limits.MaxActiveChildren = runs.MaxActiveChildRuns
	if !start.ChildRunAdmissionEnabled {
		limits.MaxDepth = 1
		limits.MaxActiveChildren = 1
	}
	return limits
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
		retained = &toolset.Retained{SkillDiscovery: checkpoint.skillDiscoveries[group], MCP: captured, Bindings: checkpoint.bindings, Resources: checkpoint.resources}
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
		_, found, decodeErr := readContextSources(messages[index].Metadata)
		if decodeErr != nil {
			return nil, fmt.Errorf("execution: decode Interaction instruction[%d] provenance: %w", index, decodeErr)
		}
		if !found {
			break
		}
		instructions = append(instructions, messages[index].Clone())
	}
	return instructions, nil
}
