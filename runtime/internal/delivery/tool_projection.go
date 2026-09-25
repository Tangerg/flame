package delivery

import (
	json "encoding/json/v2"
	"fmt"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/exactjson"
)

// The public protocol represents schemas as objects. A schema states its bounds
// as numeric literals, so [exactjson.Numbers] keeps each one as written: a
// maximum or multipleOf rounded through float64 would publish a constraint the
// Tool never declared.
func presentToolSchema(definition chat.ToolDefinition) (map[string]any, error) {
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("delivery: tool definition: %w", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(definition.InputSchema, &schema, exactjson.Numbers()); err != nil {
		return nil, fmt.Errorf("delivery: tool schema: %w", err)
	}
	return schema, nil
}
