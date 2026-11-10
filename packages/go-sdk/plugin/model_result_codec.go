package plugin

import (
	json "encoding/json/v2"

	"github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/content"

	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
)

type modelResultProjection struct {
	Response *scopewire.Response `json:"response,omitzero" jsonschema:"anyof_required=response"`
	Failure  *ModelFailure       `json:"failure,omitzero" jsonschema:"anyof_required=failure"`
}

func (ModelResult) JSONSchemaModel() any { return modelResultProjection{} }
func (r ModelResult) MarshalJSON() ([]byte, error) {
	type native ModelResult
	return json.Marshal(native(r))
}
func (r *ModelResult) UnmarshalJSON(data []byte) error {
	var encoded modelResultProjection
	if err := value.DecodeValue(data, &encoded, "model result"); err != nil {
		return err
	}
	*r = ModelResult{Response: (*chat.Response)(encoded.Response), Failure: encoded.Failure}
	return nil
}

type modelContextResultProjection struct {
	Content []scopewire.Block `json:"content"`
}

func (ModelContextResult) JSONSchemaModel() any { return modelContextResultProjection{} }
func (r ModelContextResult) MarshalJSON() ([]byte, error) {
	type native ModelContextResult
	return json.Marshal(native(r))
}
func (r *ModelContextResult) UnmarshalJSON(data []byte) error {
	var encoded modelContextResultProjection
	if err := value.DecodeValue(data, &encoded, "model context result"); err != nil {
		return err
	}
	blocks := make([]content.Block, len(encoded.Content))
	for index, block := range encoded.Content {
		blocks[index] = content.Block(block)
	}
	*r = ModelContextResult{Content: blocks}
	return nil
}
