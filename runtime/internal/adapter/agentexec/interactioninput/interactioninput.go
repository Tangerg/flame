// Package interactioninput translates product Interrupt values to Agent Framework
// Interaction pending inputs and semantic response Signals. Agent Framework owns the
// wait lifecycle; this package owns only the anti-corruption boundary.
package interactioninput

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
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	"github.com/Tangerg/scope/core/jsonschema"
)

// resolutionSchema derives the response contract from the payload type this
// package decodes, so the shape the Agent Framework enforces at the wait
// boundary cannot drift from the shape that is read back. Which remember scopes
// exist stays with approval.Scope, which rejects an unknown one at decode.
var resolutionSchema = sync.OnceValues(func() (jsontext.Value, error) {
	schema, err := jsonschema.For[ResolutionPayload]()
	if err != nil {
		return nil, fmt.Errorf("agentexec interaction input: derive resolution schema: %w", err)
	}
	return jsontext.Value(schema.JSON()), nil
})

type capabilityContextKey struct{}

type capabilityPolicy struct {
	allowed []interrupt.Kind
}

// WithCapabilities freezes the product input kinds an Interaction Tool may
// request. Require fails closed when a Tool asks for an unadmitted kind.
func WithCapabilities(ctx context.Context, allowed []interrupt.Kind) context.Context {
	return context.WithValue(ctx, capabilityContextKey{}, capabilityPolicy{
		allowed: slices.Clone(allowed),
	})
}

type continuationWire struct {
	Key          string         `json:"key"`
	PromptDigest string         `json:"prompt_digest"`
	Prompt       jsontext.Value `json:"prompt"`
}

// Continuation is the validated product input restored while Agent Framework re-enters
// the Tool invocation that requested it.
type Continuation struct {
	Key        string
	Interrupt  runs.Interrupt
	Resolution interrupt.Resolution
}

// Restore reads the exact prompt and response from Agent Framework's public Tool-input
// continuation context. found=false identifies an initial invocation.
func Restore(ctx context.Context) (Continuation, bool, error) {
	continuation, found := interaction.ToolInputContinuationFromContext(ctx)
	if !found {
		return Continuation{}, false, nil
	}
	state, err := decode[continuationWire](continuation.State())
	if err != nil {
		return Continuation{}, true, fmt.Errorf("agentexec interaction input: decode continuation: %w", err)
	}
	if state.Key == "" || !jsontext.Value(state.Prompt).IsValid() {
		return Continuation{}, true, errors.New("agentexec interaction input: invalid continuation identity or prompt")
	}
	prompt, err := DecodePrompt(state.Prompt)
	if err != nil {
		return Continuation{}, true, fmt.Errorf("agentexec interaction input: restore prompt: %w", err)
	}
	canonicalPrompt, err := EncodePrompt(prompt)
	if err != nil {
		return Continuation{}, true, fmt.Errorf("agentexec interaction input: normalize restored prompt: %w", err)
	}
	if state.PromptDigest != promptDigest(canonicalPrompt) {
		return Continuation{}, true, errors.New("agentexec interaction input: continuation prompt digest differs")
	}
	resolution, err := DecodeResolution(continuation.Response())
	if err != nil {
		return Continuation{}, true, err
	}
	return Continuation{Key: state.Key, Interrupt: prompt, Resolution: resolution}, true, nil
}

// Require returns a restored decision at the original call site or requests a
// new Agent Framework Interaction input.
func Require(ctx context.Context, key string, prompt runs.Interrupt) (interrupt.Resolution, error) {
	promptJSON, err := admitRequirement(ctx, key, prompt)
	if err != nil {
		return interrupt.Resolution{}, err
	}
	continued, restored, err := Restore(ctx)
	if err != nil {
		return interrupt.Resolution{}, err
	}
	if restored {
		return restoredResolution(continued, key, promptJSON)
	}
	stateJSON, err := encodeRequirementState(key, promptJSON)
	if err != nil {
		return interrupt.Resolution{}, err
	}
	schema, err := resolutionSchema()
	if err != nil {
		return interrupt.Resolution{}, err
	}
	return interrupt.Resolution{}, interaction.RequireToolInput(promptJSON, schema, stateJSON)
}

func admitRequirement(ctx context.Context, key string, prompt runs.Interrupt) (jsontext.Value, error) {
	promptJSON, err := EncodePrompt(prompt)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("agentexec interaction input: request key is required")
	}
	policy, ok := capabilityPolicyFrom(ctx)
	if !ok {
		return nil, errors.New("agentexec interaction input: Run capabilities are unavailable")
	}
	if !slices.Contains(policy.allowed, prompt.Kind) {
		return nil, fmt.Errorf(
			"agentexec interaction input: %s is outside the Run capability set",
			prompt.Kind,
		)
	}
	return promptJSON, nil
}

func restoredResolution(
	continued Continuation,
	key string,
	promptJSON jsontext.Value,
) (interrupt.Resolution, error) {
	if continued.Key != key {
		return interrupt.Resolution{}, errors.New("agentexec interaction input: continuation addresses another request")
	}
	storedJSON, err := EncodePrompt(continued.Interrupt)
	if err != nil || !bytes.Equal(storedJSON, promptJSON) {
		return interrupt.Resolution{}, errors.New("agentexec interaction input: prompt changed during continuation")
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
		return nil, fmt.Errorf("agentexec interaction input: encode continuation: %w", err)
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
