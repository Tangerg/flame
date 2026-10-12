package execution

import (
	"slices"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
)

func (i *interactionSession) checkpointMCPReferences() ([]string, error) {
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	references := make([]string, 0)
	if deployments == nil {
		return references, nil
	}
	for _, manifest := range deployments.manifests {
		for _, executable := range append(slices.Clone(manifest.Visible), manifest.Deferred...) {
			ref, err := toolset.Identify(executable)
			if err != nil {
				return nil, err
			}
			if _, _, mcp := ref.MCP(); mcp && !slices.Contains(references, ref.String()) {
				references = append(references, ref.String())
			}
		}
	}
	slices.Sort(references)
	return references, nil
}

func pendingCheckpointSources(session *interactionSession, checkpoint interactionCheckpointState) bool {
	session.state.mu.Lock()
	deployments := session.state.deployments
	session.state.mu.Unlock()
	if deployments == nil {
		return false
	}
	for _, manifest := range deployments.manifests {
		for _, source := range manifest.PendingMCP {
			if slices.Contains(checkpoint.mcpSources, source) {
				return true
			}
		}
	}
	return false
}
