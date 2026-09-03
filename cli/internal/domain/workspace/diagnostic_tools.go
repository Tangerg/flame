package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type DiagnosticToolDescriptor struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

func (d DiagnosticToolDescriptor) Validate() error {
	name := strings.TrimSpace(d.Name)
	if name == "" {
		return errors.New("diagnostic tool name is empty")
	}
	if name != d.Name {
		return errors.New("diagnostic tool name is not canonical")
	}
	return validateObject("diagnostic tool schema", d.Schema)
}

func (d DiagnosticToolDescriptor) Clone() DiagnosticToolDescriptor {
	d.Schema = append(json.RawMessage(nil), d.Schema...)
	return d
}

type DiagnosticToolInvocation struct {
	Tool      DiagnosticToolDescriptor
	Arguments json.RawMessage
	Workspace string
}

func (i DiagnosticToolInvocation) Validate() error {
	if err := i.Tool.Validate(); err != nil {
		return fmt.Errorf("diagnostic tool invocation: %w", err)
	}
	if strings.TrimSpace(i.Workspace) == "" {
		return errors.New("diagnostic tool invocation workspace is empty")
	}
	return validateObject("diagnostic tool arguments", i.Arguments)
}

type DiagnosticToolResult struct{ JSON json.RawMessage }

func (r DiagnosticToolResult) Validate() error {
	if len(r.JSON) == 0 || !json.Valid(r.JSON) {
		return errors.New("diagnostic tool result is not valid JSON")
	}
	return nil
}

func (r DiagnosticToolResult) Clone() DiagnosticToolResult {
	return DiagnosticToolResult{JSON: append(json.RawMessage(nil), r.JSON...)}
}

// ParseDiagnosticToolArguments owns the direct-invocation JSON-object invariant without
// requiring delivery code to manufacture a partial tool descriptor merely to
// validate user input.
func ParseDiagnosticToolArguments(value string) (json.RawMessage, error) {
	arguments := json.RawMessage(strings.TrimSpace(value))
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if err := validateObject("diagnostic tool arguments", arguments); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), arguments...), nil
}

func validateObject(name string, value json.RawMessage) error {
	if len(value) == 0 || !json.Valid(value) {
		return fmt.Errorf("%s is not valid JSON", name)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value, &object); err != nil || object == nil {
		return fmt.Errorf("%s must be a JSON object", name)
	}
	return nil
}
