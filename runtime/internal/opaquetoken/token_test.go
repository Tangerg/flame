package opaquetoken

import (
	"errors"
	"strings"
	"testing"
)

const testMaximumTokenCharacters = 128

type testPayload struct {
	Value string `json:"value"`
}

func TestRoundTrip(t *testing.T) {
	token, err := Encode(testPayload{Value: "expected"}, testMaximumTokenCharacters)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var got testPayload
	if err := Decode(token, testMaximumTokenCharacters, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Value != "expected" {
		t.Fatalf("value = %q, want expected", got.Value)
	}
}

func TestDecodeRejectsUnknownFieldsAndWrongShapes(t *testing.T) {
	for name, payload := range map[string]any{
		"unknown field": struct {
			Value string `json:"value"`
			Extra bool   `json:"extra"`
		}{Value: "expected", Extra: true},
		"wrong shape": []string{"expected"},
	} {
		t.Run(name, func(t *testing.T) {
			token, err := Encode(payload, testMaximumTokenCharacters)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			var target testPayload
			if err := Decode(token, testMaximumTokenCharacters, &target); err == nil {
				t.Fatal("decode accepted invalid payload")
			}
		})
	}
}

func TestResourceEnvelopeIsRequiredBeforeBase64Allocation(t *testing.T) {
	token, err := Encode(testPayload{Value: "expected"}, testMaximumTokenCharacters)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Encode(testPayload{Value: "expected"}, len(token)-1); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("undersized Encode error = %v, want ErrTooLarge", err)
	}
	var target testPayload
	if err := Decode(strings.Repeat("a", testMaximumTokenCharacters+1), testMaximumTokenCharacters, &target); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized Decode error = %v, want ErrTooLarge", err)
	}
	if _, err := Encode(testPayload{}, 0); err == nil {
		t.Fatal("Encode accepted a missing resource envelope")
	}
	if err := Decode(token, 0, &target); err == nil {
		t.Fatal("Decode accepted a missing resource envelope")
	}
}
