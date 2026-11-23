package tool

import (
	"errors"
	"fmt"
	"unicode/utf8"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

var errInvalidDefinition = errors.New("tool: invalid definition")

type Group string

const (
	GroupRoot      Group = "root"
	GroupDelegated Group = "delegated"
)

func (g Group) Valid() bool { return g == GroupRoot || g == GroupDelegated }

type Tool struct {
	toolcontract.Definition
	SafetyClass SafetyClass
}

func (t Tool) Validate() error {
	if err := t.Definition.Validate(); err != nil {
		return fmt.Errorf("%w: %w", errInvalidDefinition, err)
	}
	if !utf8.ValidString(t.Description) {
		return fmt.Errorf("%w: Tool %q description is not valid UTF-8", errInvalidDefinition, t.Name)
	}
	if !t.SafetyClass.Valid() {
		return fmt.Errorf("%w: Tool %q has unknown safety class %q", errInvalidDefinition, t.Name, t.SafetyClass)
	}
	return nil
}

type SafetyClass string

const (
	// Network-reaching tools are not safe even when they only read remote state.
	SafetyClassSafe    SafetyClass = "safe"
	SafetyClassWrite   SafetyClass = "write"
	SafetyClassExec    SafetyClass = "exec"
	SafetyClassNetwork SafetyClass = "network"
)
