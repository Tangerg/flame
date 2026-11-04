package modelprovider

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"

	"github.com/Tangerg/flame/packages/go-sdk/model"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame/value"
)

// Scope owns each complete catalog model, including its time and pricing
// codecs. Flame declares only the provider policy around that native envelope.
type profileProjection struct {
	ID               string               `json:"id"`
	Authentication   Authentication       `json:"authentication"`
	Endpoint         model.EndpointPolicy `json:"endpoint,omitzero"`
	EndpointRequired bool                 `json:"endpointRequired"`
	ModelSource      ModelSource          `json:"modelSource"`
	DefaultModel     string               `json:"defaultModel,omitempty"`
	Embedding        *EmbeddingProfile    `json:"embedding,omitzero"`
	Models           []jsontext.Value     `json:"models"`
}

func (Profile) JSONSchemaModel() any { return profileProjection{} }

func (p Profile) MarshalJSON() ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	type native Profile
	return json.Marshal(native(p))
}

func (p *Profile) UnmarshalJSON(data []byte) error {
	var shape profileProjection
	if err := value.DecodeValue(data, &shape, "model provider profile"); err != nil {
		return err
	}
	type native Profile
	var decoded native
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	candidate := Profile(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*p = candidate
	return nil
}

type chatProjection struct {
	Handle
	Original      pluginmcp.CommandID `json:"original"`
	Provider      string              `json:"provider"`
	Model         string              `json:"model"`
	Streaming     bool                `json:"streaming"`
	TokenCounting bool                `json:"tokenCounting"`
	Metadata      jsontext.Value      `json:"metadata,omitzero"`
}

func (Chat) JSONSchemaModel() any { return chatProjection{} }

func (c Chat) MarshalJSON() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	type native Chat
	return json.Marshal(native(c))
}

func (c *Chat) UnmarshalJSON(data []byte) error {
	var shape chatProjection
	if err := value.DecodeValue(data, &shape, "model provider chat handle"); err != nil {
		return err
	}
	type native Chat
	var decoded native
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	candidate := Chat(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*c = candidate
	return nil
}
