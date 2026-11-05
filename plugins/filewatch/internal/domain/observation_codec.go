package filewatch

import (
	json "encoding/json/v2"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/frame/process/mcp"
	"github.com/Tangerg/frame/value"
)

type captureProjection struct {
	Generation string            `json:"generation"`
	Sequence   uint64            `json:"sequence"`
	Revision   int64             `json:"revision"`
	Kind       Kind              `json:"kind"`
	Paths      []string          `json:"paths"`
	ObservedAt instant.Timestamp `json:"observedAt" jsonschema:"format=date-time"`
	Trigger    Draft             `json:"trigger"`
	SessionID  string            `json:"sessionId"`
	RunID      string            `json:"runId"`
	Invocation mcp.Invocation    `json:"invocation"`
}

func (Capture) JSONSchemaModel() any { return captureProjection{} }

func (c Capture) MarshalJSON() ([]byte, error) {
	type native Capture
	return json.Marshal(native(c))
}

func (c *Capture) UnmarshalJSON(data []byte) error {
	var projection captureProjection
	if err := value.DecodeValue(data, &projection, "file observation"); err != nil {
		return err
	}
	type native Capture
	var decoded native
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	*c = Capture(decoded)
	return nil
}

type headerProjection struct {
	ID         string            `json:"id"`
	Revision   int64             `json:"revision"`
	Kind       Kind              `json:"kind"`
	ObservedAt instant.Timestamp `json:"observedAt" jsonschema:"format=date-time"`
	PathCount  int               `json:"pathCount"`
	SessionID  string            `json:"sessionId"`
	RunID      string            `json:"runId"`
	Accepted   bool              `json:"accepted"`
}

func (Header) JSONSchemaModel() any { return headerProjection{} }

func (h Header) MarshalJSON() ([]byte, error) {
	type native Header
	return json.Marshal(native(h))
}

func (h *Header) UnmarshalJSON(data []byte) error {
	var projection headerProjection
	if err := value.DecodeValue(data, &projection, "file observation header"); err != nil {
		return err
	}
	type native Header
	var decoded native
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	*h = Header(decoded)
	return nil
}
