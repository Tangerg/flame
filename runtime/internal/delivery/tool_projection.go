package delivery

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
)

// The public protocol represents schemas as objects. Keep numeric literals
// exact when projecting Scope's definition into that wire representation.
// Exact numeric literals require encoding/json's UseNumber: decoding a JSON
// number into an any through encoding/json/v2 yields float64, which silently
// rounds identifiers beyond IEEE-754's exact integer range. The rest of
// Runtime uses encoding/json/v2.
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
