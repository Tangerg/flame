package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func (i *interactionSession) checkpointMCPManifests() (map[domaintool.Group][]string, error) {
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	if deployments == nil {
		return nil, errors.New("execution: checkpoint Tool manifests are unavailable")
	}
	manifests := make(map[domaintool.Group][]string, len(deployments.manifests))
	for group, manifest := range deployments.manifests {
		references := make([]string, 0)
		for _, executable := range append(slices.Clone(manifest.Visible), manifest.Deferred...) {
			ref, err := toolset.Identify(executable)
			if err != nil {
				return nil, err
			}
			if _, _, mcp := ref.MCP(); mcp && !slices.Contains(references, ref.String()) {
				references = append(references, ref.String())
			}
		}
		slices.Sort(references)
		manifests[group] = references
	}
	return manifests, nil
}

func decodeMCPManifests(values map[domaintool.Group][]string) (map[domaintool.Group][]domaintool.Ref, error) {
	if _, found := values[domaintool.GroupRoot]; !found {
		return nil, errors.New("execution: checkpoint root Tool manifest is required")
	}
	manifests := make(map[domaintool.Group][]domaintool.Ref, len(values))
	for group, texts := range values {
		if !group.Valid() || texts == nil {
			return nil, errors.New("execution: checkpoint Tool group and references are required")
		}
		references := make([]domaintool.Ref, 0, len(texts))
		for _, text := range texts {
			ref, err := domaintool.ParseRef(text)
			if err != nil {
				return nil, err
			}
			if _, _, mcp := ref.MCP(); !mcp || slices.Contains(references, ref) {
				return nil, errors.New("execution: checkpoint requires distinct MCP Tool references")
			}
			references = append(references, ref)
		}
		manifests[group] = references
	}
	return manifests, nil
}

func pendingCheckpointSources(session *interactionSession, checkpoint interactionCheckpointState) bool {
	session.state.mu.Lock()
	deployments := session.state.deployments
	session.state.mu.Unlock()
	if deployments == nil {
		return false
	}
	for group, manifest := range deployments.manifests {
		for _, ref := range checkpoint.mcpManifests[group] {
			source, _, _ := ref.MCP()
			if slices.Contains(manifest.PendingMCP, source) {
				return true
			}
		}
	}
	return false
}

func (i *interactionSession) skillDiscoveryCommands() map[domaintool.Group]string {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	var commands map[domaintool.Group]string
	if i.state.deployments != nil {
		for group, manifest := range i.state.deployments.manifests {
			if manifest.SkillDiscovery != "" {
				if commands == nil {
					commands = make(map[domaintool.Group]string)
				}
				commands[group] = manifest.SkillDiscovery
			}
		}
	}
	return commands
}

func decodeCapturedToolGroups(mcp map[domaintool.Group][]string, skills map[domaintool.Group]string) (map[domaintool.Group][]domaintool.Ref, error) {
	for group, command := range skills {
		if _, ok := mcp[group]; !ok || !validSkillCapture(command) {
			return nil, errors.New("execution: invalid captured Skill discovery")
		}
	}
	return decodeMCPManifests(mcp)
}

func validSkillCapture(command string) bool {
	digest, err := hex.DecodeString(command)
	return err == nil && len(digest) == sha256.Size && command == strings.ToLower(command)
}
