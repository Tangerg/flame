package delivery

import (
	json "encoding/json/v2"
	"fmt"

	framevalue "github.com/Tangerg/frame/value"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// framevalue.Numbers keeps schema bounds as written: a maximum rounded through
// float64 would publish a constraint the Tool never declared.
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
