package modelprovider

import (
	json "encoding/json/v2"

	"github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
)

type chatRequestProjection struct {
	Handle  Handle            `json:"handle"`
	Request scopewire.Request `json:"request"`
}

func (ChatRequest) JSONSchemaModel() any { return chatRequestProjection{} }
func (r ChatRequest) MarshalJSON() ([]byte, error) {
	type native ChatRequest
	return json.Marshal(native(r))
}
func (r *ChatRequest) UnmarshalJSON(data []byte) error {
	var encoded chatRequestProjection
	if err := value.DecodeValue(data, &encoded, "provider chat request"); err != nil {
		return err
	}
	*r = ChatRequest{Handle: encoded.Handle, Request: chat.Request(encoded.Request)}
	return nil
}

type chatResultProjection struct {
	Response *scopewire.Response `json:"response,omitzero"`
	Failure  *Failure            `json:"failure,omitzero"`
}

func (ChatResult) JSONSchemaModel() any { return chatResultProjection{} }
func (r ChatResult) MarshalJSON() ([]byte, error) {
	type native ChatResult
	return json.Marshal(native(r))
}
func (r *ChatResult) UnmarshalJSON(data []byte) error {
	var encoded chatResultProjection
	if err := value.DecodeValue(data, &encoded, "provider chat result"); err != nil {
		return err
	}
	*r = ChatResult{Response: (*chat.Response)(encoded.Response), Failure: encoded.Failure}
	return nil
}
