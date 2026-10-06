package testsupport

import (
	"crypto/sha256"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

// MustAgentMemoryItem fills irrelevant identity, partition, provenance and
// lifecycle defaults, restores snapshot, and panics when the fixture is invalid.
// An omitted ID is derived from the content so fixtures stay deterministic.
func MustAgentMemoryItem(snapshot agentmemory.ItemSnapshot) agentmemory.Item {
	if snapshot.ID == (agentmemory.ItemID{}) {
		var entropy [agentmemory.ItemIDEntropyBytes]byte
		digest := sha256.Sum256([]byte(snapshot.Content))
		copy(entropy[:], digest[:])
		snapshot.ID = agentmemory.NewItemID(entropy)
	}
	if snapshot.Scope == "" {
		snapshot.Scope = agentmemory.ScopeProject
	}
	if snapshot.Scope == agentmemory.ScopeProject && snapshot.Project == "" {
		snapshot.Project = "/repo"
	}
	if snapshot.Status == "" {
		snapshot.Status = agentmemory.StatusActive
	}
	if snapshot.Origin == "" {
		snapshot.Origin = agentmemory.OriginUser
		if snapshot.Status != agentmemory.StatusActive {
			snapshot.Origin = agentmemory.OriginAuto
		}
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Unix(1, 0).UTC()
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = snapshot.CreatedAt
	}
	item, err := agentmemory.RestoreItem(snapshot)
	if err != nil {
		panic(err)
	}
	return item
}
