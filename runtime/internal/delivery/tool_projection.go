package delivery

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
)

// The public protocol represents schemas as objects. Keep numeric literals
// exact when projecting Scope's definition into that wire representation.
func presentToolSchema(definition chat.ToolDefinition) (map[string]any, error) {
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("delivery: tool definition: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(definition.InputSchema))
	decoder.UseNumber()
	var schema map[string]any
	if err := decoder.Decode(&schema); err != nil {
		return nil, fmt.Errorf("delivery: tool schema: %w", err)
	}
	return schema, nil
}
