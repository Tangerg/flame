package modelref

import (
	"bytes"
	json "encoding/json/v2"
	"fmt"
)

func (s Selection) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"provider": s.Provider(), "model": s.Model(), "reasoningEffort": s.ReasoningEffort()}, json.Deterministic(true))
}
func (s *Selection) UnmarshalJSON(data []byte) error {
	var encoded map[string]*string
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("model selection: null is not a selection")
	}
	if err := json.Unmarshal(data, &encoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if len(encoded) != 3 {
		return fmt.Errorf("model selection: provider, model and reasoningEffort are required")
	}
	for key, value := range encoded {
		if key != "provider" && key != "model" && key != "reasoningEffort" {
			return fmt.Errorf("model selection: unknown member %q", key)
		}
		if value == nil {
			return fmt.Errorf("model selection: member %q cannot be null", key)
		}
	}
	value, err := NewWithReasoningEffort(*encoded["provider"], *encoded["model"], *encoded["reasoningEffort"])
	if err == nil {
		*s = value
	}
	return err
}
