package modelref

import (
	"bytes"
	json "encoding/json/v2"
	"fmt"
)

// Schema emission and presence-aware decoding share the same closed field set.
type selectionJSON[T any] struct {
	Model           T `json:"model"`
	Provider        T `json:"provider"`
	ReasoningEffort T `json:"reasoningEffort"`
}

func (s Selection) MarshalJSON() ([]byte, error) {
	return json.Marshal(selectionJSON[string]{Provider: s.Provider(), Model: s.Model(), ReasoningEffort: s.ReasoningEffort()}, json.Deterministic(true))
}
func (s *Selection) UnmarshalJSON(data []byte) error {
	var encoded selectionJSON[*string]
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("model selection: null is not a selection")
	}
	if err := json.Unmarshal(data, &encoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if encoded.Provider == nil || encoded.Model == nil || encoded.ReasoningEffort == nil {
		return fmt.Errorf("model selection: provider, model and reasoningEffort are required")
	}
	value, err := NewWithReasoningEffort(*encoded.Provider, *encoded.Model, *encoded.ReasoningEffort)
	if err == nil {
		*s = value
	}
	return err
}

func (Selection) JSONSchemaAlias() any { return selectionJSON[string]{} }
