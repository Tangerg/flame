package rpc

//go:generate sh -c "go run github.com/Tangerg/flame/plugins/sandbox/backend/cmd/contracts | go run github.com/Tangerg/flame/runtime/cmd/contractgen --feature - --out ../../package/io.github.tangerg.flame/contracts --ts ../../package/io.github.tangerg.flame/contracts"

import (
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	sandbox "github.com/Tangerg/flame/plugins/sandbox/backend/domain"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/idempotency"
)

type ExecuteInput struct {
	Script        string `json:"script" jsonschema:"minLength=1,maxLength=65536"`
	TimeoutMillis int    `json:"timeoutMillis" jsonschema:"minimum=1,maximum=30000"`
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
	Capability *capability.Contract
	Execute    *pluginsdk.Method[ExecuteInput, sandbox.Result]
	Limits     *pluginsdk.Method[struct{}, Limits]
}

func NewContracts() (Contracts, error) {
	var c Contracts
	var err error
	c.Execute, err = pluginsdk.NewMethod[ExecuteInput, sandbox.Result](pluginsdk.MethodConfig{Name: "flame.sandbox/execute.v1", Kind: capability.Mutation, Target: "installation", Description: "Execute a bounded /bin/sh script in a new private native sandbox workspace. Writes stay in that workspace, the configured home is hidden, network is denied and child environment is scrubbed. No CPU or memory quota is promised. Replaying the original command returns its saved outcome; unknown work is never re-executed."})
	if err != nil {
		return Contracts{}, err
	}
	c.Limits, err = pluginsdk.NewMethod[struct{}, Limits](pluginsdk.MethodConfig{Name: "flame.sandbox/limits.v1", Kind: capability.Query, Target: "installation", Description: "Inspect native backend availability and the actual script, timeout and retained-output limits. Availability does not substitute for successful confinement at execution."})
	if err != nil {
		return Contracts{}, err
	}
	c.Capability, err = capability.New(capability.Config{Reference: "flame.sandbox.v1", Composition: capability.Collection, Scope: "installation"}, c.Execute.Descriptor(), c.Limits.Descriptor())
	return c, err
}

func Classify(err error) string {
	switch {
	case errors.Is(err, sandbox.ErrInvalidProgram):
		return "invalid_program"
	case errors.Is(err, idempotency.ErrKeyConflict):
		return "command_conflict"
	default:
		return ""
	}
}
