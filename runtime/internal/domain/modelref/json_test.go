package modelref

import (
	json "encoding/json/v2"
	"testing"
)

func TestSelectionJSONPreservesExactIdentityAndDefault(t *testing.T) {
	exact, err := NewWithReasoningEffort("openai", "gpt-5", "high")
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range []Selection{{}, exact} {
		encoded, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var restored Selection
		if err := json.Unmarshal(encoded, &restored); err != nil || !restored.Equal(original) {
			t.Fatalf("round trip %s: %v %v", encoded, restored, err)
		}
	}
}

func TestSelectionJSONRefusesMalformedCaptureWithoutChangingIdentity(t *testing.T) {
	original, err := New("openai", "gpt-5")
	if err != nil {
		t.Fatal(err)
	}
	for _, encoded := range []string{
		`null`,
		`{}`,
		`{"provider":"openai","model":"gpt-5"}`,
		`{"provider":"openai","model":"gpt-5","unknown":""}`,
		`{"provider":"openai","model":null,"reasoningEffort":""}`,
		`{"provider":"openai","provider":"openai","model":"gpt-5","reasoningEffort":""}`,
		`{"provider":"","model":"gpt-5","reasoningEffort":""}`,
		`{"provider":"","model":"","reasoningEffort":"high"}`,
	} {
		t.Run(encoded, func(t *testing.T) {
			value := original
			if err := json.Unmarshal([]byte(encoded), &value); err == nil {
				t.Fatal("malformed capture accepted")
			}
			if !value.Equal(original) {
				t.Fatal("failed decode changed identity")
			}
		})
	}
}
