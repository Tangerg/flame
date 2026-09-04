package tool

import (
	"errors"
	"testing"
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
	valid := Tool{Name: "read", Description: "Read a file", SafetyClass: SafetyClassSafe}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Tool.Validate() error = %v", err)
	}

	for name, candidate := range map[string]Tool{
		"empty name":          {SafetyClass: SafetyClassSafe},
		"padded name":         {Name: " read ", SafetyClass: SafetyClassSafe},
		"invalid name":        {Name: string([]byte{0xff}), SafetyClass: SafetyClassSafe},
		"invalid description": {Name: "read", Description: string([]byte{0xff}), SafetyClass: SafetyClassSafe},
		"unknown safety":      {Name: "read", SafetyClass: SafetyClass("future")},
	} {
		t.Run(name, func(t *testing.T) {
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidDefinition) {
				t.Fatalf("Tool.Validate() error = %v, want ErrInvalidDefinition", err)
			}
		})
	}
}
