// Package input translates product Interrupt values to Scope's Tool input
// contract. Scope owns the suspension, wait, and re-entry; this package owns
// only the anti-corruption boundary between a product Interrupt and Scope's
// InputRequest and Continuation.
package input

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/jsonschema"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// resolutionSchema derives the response contract from the payload type this
// package decodes, so the shape Scope enforces at the wait boundary cannot
// drift from the shape that is read back. Which remember scopes exist stays
// with approval.Scope, which rejects an unknown one at decode.
var resolutionSchema = sync.OnceValues(func() (jsonschema.Schema, error) {
	schema, err := jsonschema.For[resolutionPayload]()
	if err != nil {
		return jsonschema.Schema{}, fmt.Errorf("run input: derive resolution schema: %w", err)
	}
	return schema, nil
})

type capabilityContextKey struct{}

type capabilityPolicy struct {
	allowed []run.InterruptKind
}

// WithCapabilities freezes the product input kinds an Interaction Tool may
// request. Require fails closed when a Tool asks for an unadmitted kind.
func WithCapabilities(ctx context.Context, allowed []run.InterruptKind) context.Context {
	return context.WithValue(ctx, capabilityContextKey{}, capabilityPolicy{
		allowed: slices.Clone(allowed),
	})
}

type continuationWire struct {
	Key          string         `json:"key"`
	PromptDigest string         `json:"prompt_digest"`
	Prompt       jsontext.Value `json:"prompt"`
}

// Continuation is the validated product input Scope restored while re-entering
// the Tool invocation that requested it.
type Continuation struct {
	Key        string
	Interrupt  runs.Interrupt
	Resolution run.InterruptResolution
}

// Restore decodes Scope's re-entry continuation into the exact prompt and
// validated resolution it froze. found=false identifies an initial invocation.
func Restore(invocation toolcontract.Invocation) (Continuation, bool, error) {
	continuation, found := invocation.Continuation()
	if !found {
		return Continuation{}, false, nil
	}
	state, err := decode[continuationWire](continuation.State())
	if err != nil {
		return Continuation{}, true, fmt.Errorf("run input: decode continuation: %w", err)
	}
	if state.Key == "" || !jsontext.Value(state.Prompt).IsValid() {
		return Continuation{}, true, errors.New("run input: invalid continuation identity or prompt")
	}
	prompt, err := DecodePrompt(state.Prompt)
	if err != nil {
		return Continuation{}, true, fmt.Errorf("run input: restore prompt: %w", err)
	}
	canonicalPrompt, err := EncodePrompt(prompt)
	if err != nil {
		return Continuation{}, true, fmt.Errorf("run input: normalize restored prompt: %w", err)
	}
	if state.PromptDigest != promptDigest(canonicalPrompt) {
		return Continuation{}, true, errors.New("run input: continuation prompt digest differs")
	}
	resolution, err := decodeResolution(continuation.Response())
	if err != nil {
		return Continuation{}, true, err
	}
	if resolution.Kind() != prompt.Kind {
		return Continuation{}, true, errors.New("run input: answer kind differs from the persisted prompt")
	}
	return Continuation{Key: state.Key, Interrupt: prompt, Resolution: resolution}, true, nil
}

// Require returns a restored decision for an already-answered request, or a
// Scope InputRequest the Tool returns as tool.Result{InputRequest} to park on
// new input. Scope owns the suspension, wait identity, and re-entry.
func Require(ctx context.Context, invocation toolcontract.Invocation, key string, prompt runs.Interrupt) (run.InterruptResolution, *toolcontract.InputRequest, error) {
	promptJSON, err := admitRequirement(ctx, key, prompt)
	if err != nil {
		return run.InterruptResolution{}, nil, err
	}
	continued, restored, err := Restore(invocation)
	if err != nil {
		return run.InterruptResolution{}, nil, err
	}
	if restored {
		resolution, err := restoredResolution(continued, key, promptJSON)
		return resolution, nil, err
	}
	stateJSON, err := encodeRequirementState(key, promptJSON)
	if err != nil {
		return run.InterruptResolution{}, nil, err
	}
	schema, err := resolutionSchema()
	if err != nil {
		return run.InterruptResolution{}, nil, err
	}
	request, err := toolcontract.NewInputRequest(toolcontract.InputRequestConfig{
		Prompt:            []byte(promptJSON),
		ResponseSchema:    schema,
		ContinuationState: []byte(stateJSON),
	})
	if err != nil {
		return run.InterruptResolution{}, nil, fmt.Errorf("run input: build input request: %w", err)
	}
	return run.InterruptResolution{}, &request, nil
}

func admitRequirement(ctx context.Context, key string, prompt runs.Interrupt) (jsontext.Value, error) {
	promptJSON, err := EncodePrompt(prompt)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("run input: request key is required")
	}
	policy, ok := capabilityPolicyFrom(ctx)
	if !ok {
		return nil, errors.New("run input: Run capabilities are unavailable")
	}
	if !slices.Contains(policy.allowed, prompt.Kind) {
		return nil, fmt.Errorf(
			"run input: %s is outside the Run capability set",
			prompt.Kind,
		)
	}
	return promptJSON, nil
}

func restoredResolution(
	continued Continuation,
	key string,
	promptJSON jsontext.Value,
) (run.InterruptResolution, error) {
	if continued.Key != key {
		return run.InterruptResolution{}, errors.New("run input: continuation addresses another request")
	}
	storedJSON, err := EncodePrompt(continued.Interrupt)
	if err != nil || !bytes.Equal(storedJSON, promptJSON) {
		return run.InterruptResolution{}, errors.New("run input: prompt changed during continuation")
	}
	return continued.Resolution, nil
}

func encodeRequirementState(key string, promptJSON jsontext.Value) (jsontext.Value, error) {
	stateJSON, err := agent.EncodePayload(continuationWire{
		Key:          key,
		PromptDigest: promptDigest(promptJSON),
		Prompt:       promptJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("run input: encode continuation: %w", err)
	}
	return stateJSON.JSON(), nil
}

func capabilityPolicyFrom(ctx context.Context) (capabilityPolicy, bool) {
	if ctx == nil {
		return capabilityPolicy{}, false
	}
	policy, ok := ctx.Value(capabilityContextKey{}).(capabilityPolicy)
	return policy, ok
}

func promptDigest(prompt jsontext.Value) string {
	return agent.ComputeDigest(prompt).String()
}
