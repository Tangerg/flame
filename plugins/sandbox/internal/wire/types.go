package wire

import (
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
	"github.com/Tangerg/scope/core/jsonschema"
)

type ExecuteInput struct {
	Script        string `json:"script" jsonschema:"minLength=1"`
	TimeoutMillis int    `json:"timeoutMillis" jsonschema:"minimum=1"`
}

type Limits struct {
	Available              bool   `json:"available"`
	Platform               string `json:"platform"`
	MaxScriptBytes         int    `json:"maxScriptBytes"`
	MaxTimeoutMillis       int    `json:"maxTimeoutMillis"`
	RetainedBytesPerStream int    `json:"retainedBytesPerStream"`
	CPUAndMemoryQuotas     bool   `json:"cpuAndMemoryQuotas"`
}

func (ExecuteInput) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"script":        {MaxLength: new(uint64(sandbox.MaxScriptBytes))},
		"timeoutMillis": {Maximum: new(float64(sandbox.MaxTimeoutMillis))},
	}}
}
