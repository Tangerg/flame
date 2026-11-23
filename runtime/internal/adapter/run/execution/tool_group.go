package execution

import (
	"context"
	"maps"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Depth changes a Program, not its group's captured Tool authority. Sharing this
// frozen binding also keeps receipt capability and schemas fixed across depths.
type interactionToolGroup struct {
	manifest toolset.Manifest
	tools    interaction.ToolSet
	// authority digests the group's Tool sources; the Interaction revision binds it.
	authority string
	contracts map[domaintool.Ref]toolcontract.Contract
	observed  map[domaintool.Ref]*observedInteractionTool
}

func (i *InteractionExecutor) bindInteractionToolGroup(ctx context.Context, manifest toolset.Manifest, session *interactionSession, start runs.RootExecutionStart) (interactionToolGroup, error) {
	visible, deferred, contracts, observed, err := wrapInteractionTools(ctx, manifest, session, i.config, i.policy.toolResultOffload, start)
	if err != nil {
		return interactionToolGroup{}, err
	}
	group := interactionToolGroup{manifest: manifest, contracts: contracts, observed: observed}
	if len(visible)+len(deferred) == 0 {
		return group, nil
	}
	configuration, err := i.interactionToolConfiguration(manifest, observed)
	if err != nil {
		return interactionToolGroup{}, err
	}
	group.authority = scoperun.ComputeDigest(configuration).String()
	// Scope requires a ToolSet Revision exactly when a Tool declares a concurrency policy.
	var revision string
	if slices.ContainsFunc(slices.Collect(maps.Values(observed)), func(tool *observedInteractionTool) bool { return tool.concurrent != nil }) {
		revision = i.buildID.String() + ":" + group.authority
	}
	group.tools, err = interaction.NewToolSet(interaction.ToolSetConfig{
		Revision: revision,
		Tools:    visible, DeferredTools: deferred,
		MaxConcurrentCalls: i.policy.maxConcurrentToolCalls,
	})
	if err != nil {
		return interactionToolGroup{}, err
	}
	return group, nil
}
