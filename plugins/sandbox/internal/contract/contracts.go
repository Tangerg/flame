package contract

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/sandbox/backend/cmd/contracts | go run github.com/Tangerg/flame/packages/go-sdk/cmd/flame-sdk contract --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	json "encoding/json"
	"strconv"

	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/internal/domain"
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

type Contracts struct {
	Capability *extension.Capability
	Execute    *pluginsdk.Method[ExecuteInput, sandbox.Result]
	Limits     *pluginsdk.Method[struct{}, Limits]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Execute, err = pluginsdk.NewMethod[ExecuteInput, sandbox.Result](pluginsdk.MethodConfig{Name: "flame.sandbox/execute.v1", Kind: extension.Mutation, Target: extension.ScopeInstallation, Description: "Execute a bounded /bin/sh script in a new private native sandbox workspace. Writes stay in that workspace, the configured home is hidden, network is denied and child environment is scrubbed. No CPU or memory quota is promised. Replaying the original command returns its saved outcome; unknown work is never re-executed."})
	if err != nil {
		return Contracts{}, err
	}
	c.Limits, err = pluginsdk.NewMethod[struct{}, Limits](pluginsdk.MethodConfig{Name: "flame.sandbox/limits.v1", Kind: extension.Query, Target: extension.ScopeInstallation, Description: "Inspect native backend availability and the actual script, timeout and retained-output limits. Availability does not substitute for successful confinement at execution."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = extension.NewCapability(extension.CapabilityConfig{Reference: "flame.sandbox.v1", Composition: extension.Collection, Scope: extension.ScopeInstallation}, c.Execute.Descriptor(), c.Limits.Descriptor())
	return c, err
}

func (ExecuteInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	script, _ := schema.Properties.Get("script")
	// Structural character admission precedes the program owner's byte limit.
	script.MaxLength = new(uint64(sandbox.MaxScriptBytes))
	timeout, _ := schema.Properties.Get("timeoutMillis")
	timeout.Maximum = json.Number(strconv.Itoa(sandbox.MaxTimeoutMillis))
}
