package tool

import (
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestGroupVocabulary(t *testing.T) {
	for _, group := range []Group{GroupRoot, GroupDelegated} {
		if !group.Valid() {
			t.Errorf("Group %q is invalid", group)
		}
	}
	for _, group := range []Group{"", "role", "subtask"} {
		if group.Valid() {
			t.Errorf("Group %q is valid", group)
		}
	}
}

func TestToolValidate(t *testing.T) {
	valid := Tool{
		ToolDefinition: chat.ToolDefinition{Name: "read", Description: "Read a file", InputSchema: []byte(`{"type":"object"}`)},
		SafetyClass:    SafetyClassSafe,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Tool.Validate() error = %v", err)
	}
	for name, mutate := range map[string]func(*Tool){
		"empty name":          func(value *Tool) { value.Name = "" },
		"padded name":         func(value *Tool) { value.Name = " read " },
		"invalid name":        func(value *Tool) { value.Name = string([]byte{0xff}) },
		"invalid description": func(value *Tool) { value.Description = string([]byte{0xff}) },
		"missing schema":      func(value *Tool) { value.InputSchema = nil },
		"unknown safety":      func(value *Tool) { value.SafetyClass = SafetyClass("future") },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("Tool.Validate() error = %v, want ErrInvalidDefinition", err)
			}
		})
	}
}
