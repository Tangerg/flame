package runtime

import (
	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
)

// MaximumAgentMemoryItemsPerTarget projects the domain-owned target capacity for
// complete Go collectors; it does not establish a separate management limit.
const MaximumAgentMemoryItemsPerTarget = agentmemory.MaxVisiblePerTarget
