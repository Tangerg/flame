package wire

import (
	json "encoding/json"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
	"github.com/invopop/jsonschema"
	"strconv"
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

func (ExecuteInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	script, _ := schema.Properties.Get("script")
	// Structural character admission precedes the program owner's byte limit.
	script.MaxLength = new(uint64(sandbox.MaxScriptBytes))
	timeout, _ := schema.Properties.Get("timeoutMillis")
	timeout.Maximum = json.Number(strconv.Itoa(sandbox.MaxTimeoutMillis))
}
