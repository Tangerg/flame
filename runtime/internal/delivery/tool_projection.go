package delivery

import (
	framevalue "github.com/Tangerg/frame/value"
	toolcontract "github.com/Tangerg/scope/core/tool"

	json "encoding/json/v2"
	"fmt"
)

// The public protocol represents schemas as objects. A schema states its bounds
// as numeric literals, so [framevalue.Numbers] keeps each one as written: a
// maximum or multipleOf rounded through float64 would publish a constraint the
// Tool never declared.
func presentToolSchema(definition toolcontract.Definition) (map[string]any, error) {
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("delivery: tool definition: %w", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(definition.InputSchema, &schema, framevalue.Numbers()); err != nil {
		return nil, fmt.Errorf("delivery: tool schema: %w", err)
	}
	return schema, nil
}
