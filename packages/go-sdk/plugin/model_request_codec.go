package plugin

import (
	json "encoding/json/v2"

	"github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
)

type modelCallProjection struct {
	Selection ModelSelection    `json:"selection"`
	Budget    ModelCallBudget   `json:"budget"`
	Request   scopewire.Request `json:"request"`
}

func (ModelCallRequest) JSONSchemaModel() any { return modelCallProjection{} }
func (r ModelCallRequest) MarshalJSON() ([]byte, error) {
	type native ModelCallRequest
	return json.Marshal(native(r))
}
func (r *ModelCallRequest) UnmarshalJSON(data []byte) error {
	var encoded modelCallProjection
	if err := value.DecodeValue(data, &encoded, "model call"); err != nil {
		return err
	}
	*r = ModelCallRequest{Selection: encoded.Selection, Budget: encoded.Budget, Request: chat.Request(encoded.Request)}
	return nil
}

type modelContextProjection struct {
	Use         modelchoice.Use    `json:"use"`
	Provider    string             `json:"provider"`
	Model       string             `json:"model"`
	Request     scopewire.Request  `json:"request"`
	UserMessage *scopewire.Message `json:"userMessage,omitzero"`
}

func (ModelContextRequest) JSONSchemaModel() any { return modelContextProjection{} }
func (r ModelContextRequest) MarshalJSON() ([]byte, error) {
	type native ModelContextRequest
	return json.Marshal(native(r))
}
func (r *ModelContextRequest) UnmarshalJSON(data []byte) error {
	var encoded modelContextProjection
	if err := value.DecodeValue(data, &encoded, "model context"); err != nil {
		return err
	}
	*r = ModelContextRequest{Use: encoded.Use, Provider: encoded.Provider, Model: encoded.Model, Request: chat.Request(encoded.Request), UserMessage: (*chat.Message)(encoded.UserMessage)}
	return nil
}

type modelInputProjection struct {
	Selection ModelSelection    `json:"selection"`
	Request   scopewire.Request `json:"request"`
}

func (ModelInputRequest) JSONSchemaModel() any { return modelInputProjection{} }
func (r ModelInputRequest) MarshalJSON() ([]byte, error) {
	type native ModelInputRequest
	return json.Marshal(native(r))
}
func (r *ModelInputRequest) UnmarshalJSON(data []byte) error {
	var encoded modelInputProjection
	if err := value.DecodeValue(data, &encoded, "model input"); err != nil {
		return err
	}
	*r = ModelInputRequest{Selection: encoded.Selection, Request: chat.Request(encoded.Request)}
	return nil
}

type modelSelectionProjection struct {
	Context  modelchoice.SelectionContext `json:"context"`
	Proposed ModelSelection               `json:"proposed"`
	Request  *scopewire.Request           `json:"request,omitzero"`
}

func (ModelSelectionRequest) JSONSchemaModel() any { return modelSelectionProjection{} }
func (r ModelSelectionRequest) MarshalJSON() ([]byte, error) {
	type native ModelSelectionRequest
	return json.Marshal(native(r))
}
func (r *ModelSelectionRequest) UnmarshalJSON(data []byte) error {
	var encoded modelSelectionProjection
	if err := value.DecodeValue(data, &encoded, "model selection"); err != nil {
		return err
	}
	*r = ModelSelectionRequest{Context: encoded.Context, Proposed: encoded.Proposed, Request: (*chat.Request)(encoded.Request)}
	return nil
}
