package plugin

import (
	json "encoding/json/v2"

	"github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
)

type toolResultProjection struct {
	Output  scopewire.ToolOutput `json:"output"`
	IsError bool                 `json:"isError,omitzero"`
}

func (ToolResult) JSONSchemaModel() any { return toolResultProjection{} }
func (r ToolResult) MarshalJSON() ([]byte, error) {
	type native ToolResult
	return json.Marshal(native(r))
}
func (r *ToolResult) UnmarshalJSON(data []byte) error {
	var encoded toolResultProjection
	if err := value.DecodeValue(data, &encoded, "tool result"); err != nil {
		return err
	}
	*r = ToolResult{Output: tool.Output(encoded.Output), IsError: encoded.IsError}
	return nil
}
