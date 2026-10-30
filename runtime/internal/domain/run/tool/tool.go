// Package tool defines the runtime's model-facing tool vocabulary.
package tool

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/scope/core/chat"
)

// ErrInvalidDefinition reports registered Tool metadata that cannot be used as
// one stable model- and client-facing capability definition.
var errInvalidDefinition = errors.New("tool: invalid definition")

// Group identifies the model-facing Tool surface assigned to one Interaction
// deployment layer.
type Group string

const (
	// GroupRoot is the complete product-tool surface used by the root Agent.
	GroupRoot Group = "root"
	// GroupDelegated is the bounded surface used by delegated Agents.
	GroupDelegated Group = "delegated"
)

// Valid reports whether g names one supported Tool surface.
func (g Group) Valid() bool { return g == GroupRoot || g == GroupDelegated }

// Tool projects a Scope definition with the Runtime-owned safety class.
type Tool struct {
	chat.ToolDefinition
	SafetyClass SafetyClass
}

// Validate checks the metadata invariants shared by every Tool catalog.
func (t Tool) Validate() error {
	if err := t.ToolDefinition.Validate(); err != nil {
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

// SafetyClass classifies how aggressively the runtime gates a tool call
// behind an approval prompt. Its values are also the durable vocabulary used
// by run checkpoints; the empty value is invalid rather than silently safe.
type SafetyClass string

const (
	// SafetyClassSafe — read-only, no side effects (read, grep, glob,
	// skill). Never prompts. Network-reaching tools are not safe even when they
	// only read remote state.
	SafetyClassSafe SafetyClass = "safe"
	// SafetyClassWrite — writes files in cwd. Prompts in `safe` mode.
	SafetyClassWrite SafetyClass = "write"
	// SafetyClassExec — executes arbitrary commands (Shell). Prompts
	// in `safe` and `balanced` modes.
	SafetyClassExec SafetyClass = "exec"
	// SafetyClassNetwork — reaches off-host network. Safe/Plan gate it;
	// Balanced allows explicitly configured built-ins.
	SafetyClassNetwork SafetyClass = "network"
)
