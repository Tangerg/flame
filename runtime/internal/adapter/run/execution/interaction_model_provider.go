package execution

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
)

type modelProviderWire struct {
	Binding contributionBindingWire `json:"binding"`
	Profile modelprovider.Profile   `json:"profile"`
	Chat    modelprovider.Chat      `json:"chat"`
}

func encodeModelProvider(capture *modeladapter.ProviderCapture) (jsontext.Value, error) {
	if capture == nil {
		return jsontext.Value("null"), nil
	}
	if err := capture.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(modelProviderWire{Binding: contributionBindingsWire([]plugins.ContributionBinding{capture.Binding})[0], Profile: capture.Profile, Chat: capture.Chat})
}

func decodeModelProvider(value jsontext.Value) (*modeladapter.ProviderCapture, error) {
	if len(value) == 0 {
		return nil, errors.New("execution: frozen model provider is required")
	}
	if bytes.Equal(value, []byte("null")) {
		return nil, nil
	}
	var wire modelProviderWire
	if err := json.Unmarshal(value, &wire, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	if err := errors.Join(wire.Profile.Validate(), wire.Chat.Validate()); err != nil {
		return nil, err
	}
	if wire.Profile.ID != wire.Chat.Provider {
		return nil, errors.New("execution: provider construction identity differs")
	}
	contract, err := modelprovider.Contract()
	if err != nil {
		return nil, err
	}
	bindings, err := decodeContributionBindings([]contributionBindingWire{wire.Binding}, contract)
	if err != nil {
		return nil, err
	}
	return &modeladapter.ProviderCapture{Binding: bindings[0], Profile: wire.Profile, Chat: wire.Chat}, nil
}
