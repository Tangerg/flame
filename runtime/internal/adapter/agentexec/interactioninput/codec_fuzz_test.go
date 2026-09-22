package interactioninput

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func FuzzContinuationCodec(f *testing.F) {
	prompt, err := DecodePrompt([]byte(`{"kind":"approval","approval":{"callId":"tool_approval_1","toolName":"shell","arguments":"{}","safetyClass":"exec","risk":"high"}}`))
	if err != nil {
		f.Fatal(err)
	}
	promptJSON, err := EncodePrompt(prompt)
	if err != nil {
		f.Fatal(err)
	}
	valid, err := encodeRequirementState("approval.shell", promptJSON)
	if err != nil {
		f.Fatalf("encode continuation seed: %v", err)
	}
	f.Add([]byte(valid))
	for _, seed := range [][]byte{
		[]byte(`{"Key":"approval.shell"}`),
		[]byte(`{"key":"first","key":"second"}`),
		[]byte(`{}`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		continuation, err := decode[continuationWire](raw)
		if err != nil {
			return
		}
		if continuation.Key == "" || continuation.Prompt == nil {
			return
		}
		prompt, err := DecodePrompt(continuation.Prompt)
		if err != nil {
			return
		}
		canonicalPrompt, err := EncodePrompt(prompt)
		if err != nil || !bytes.Equal(canonicalPrompt, continuation.Prompt) || continuation.PromptDigest != promptDigest(canonicalPrompt) {
			return
		}
		encoded, err := json.Marshal(continuation)
		if err != nil {
			t.Fatalf("encode decoded continuation: %v", err)
		}
		roundTripped, err := decode[continuationWire](encoded)
		if err != nil {
			t.Fatalf("decode re-encoded continuation: %v", err)
		}
		if !reflect.DeepEqual(roundTripped, continuation) {
			t.Fatalf("continuation changed across round trip: got %#v, want %#v", roundTripped, continuation)
		}
	})
}

func FuzzPromptCodec(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"kind":"approval","approval":{"callId":"tool_approval_1","toolName":"shell","arguments":"{}","safetyClass":"exec","risk":"high"}}`),
		[]byte(`{"kind":"question","question":{"toolName":"ask_user","arguments":"{}","fields":[{"prompt":"Continue?","allowCustom":true}]}}`),
		[]byte(`{"kind":"future"}`),
		[]byte(`{}`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		prompt, err := DecodePrompt(raw)
		if err != nil {
			return
		}
		encoded, err := EncodePrompt(prompt)
		if err != nil {
			t.Fatalf("encode decoded prompt: %v", err)
		}
		roundTripped, err := DecodePrompt(encoded)
		if err != nil {
			t.Fatalf("decode re-encoded prompt: %v", err)
		}
		if !reflect.DeepEqual(roundTripped, prompt) {
			t.Fatalf("prompt changed across round trip: got %#v, want %#v", roundTripped, prompt)
		}
	})
}

func FuzzResolutionCodec(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte(`{"approved":true,"arguments":"{}","remember_scope":"session"}`),
		[]byte(`{"approved":false,"reason":"not now"}`),
		[]byte(`{"approved":true,"answers":[["yes"]]}`),
		[]byte(`{"approved":true,"answers":[null]}`),
		[]byte(`{}`),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		resolution, err := DecodeResolution(raw)
		if err != nil {
			return
		}
		encoded, err := EncodeResolution(resolution)
		if err != nil {
			t.Fatalf("encode decoded resolution: %v", err)
		}
		roundTripped, err := DecodeResolution(encoded)
		if err != nil {
			t.Fatalf("decode re-encoded resolution: %v", err)
		}
		// Scope encodes nil answer slices as empty arrays; both contain no answers.
		answersEqual := slices.EqualFunc(roundTripped.Answers, resolution.Answers, slices.Equal[[]string])
		roundTripped.Answers, resolution.Answers = nil, nil
		if !answersEqual || !reflect.DeepEqual(roundTripped, resolution) {
			t.Fatalf("resolution changed across round trip: answers equal = %v, got %#v, want %#v", answersEqual, roundTripped, resolution)
		}
	})
}
