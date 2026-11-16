package coordination

import (
	json "encoding/json/v2"
	"fmt"

	agentmemory "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

func decodeMemoryFacts(response string) ([]string, error) {
	var facts []string
	if err := json.Unmarshal([]byte(response), &facts); err != nil {
		return nil, fmt.Errorf("%w: decode string array: %w", agentmemory.ErrInvalidFacts, err)
	}
	if facts == nil {
		return nil, fmt.Errorf("%w: expected a string array, got null", agentmemory.ErrInvalidFacts)
	}
	return facts, nil
}
