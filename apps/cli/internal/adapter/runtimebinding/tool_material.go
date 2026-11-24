package runtimebinding

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/scope/tools/fs"
)

func summarizeToolArguments(call *conversation.ToolCall) {
	if call.Kind == "" || call.ArgumentsText != "" {
		return
	}
	var summary string
	switch builtin.Name(call.Name) {
	case builtin.Shell:
		var input builtin.ShellArguments
		if json.Unmarshal(call.ArgumentsJSON, &input) != nil {
			return
		}
		call.Command, summary = input.Command, input.Description
	case builtin.Read:
		var input fs.ReadRequest
		if json.Unmarshal(call.ArgumentsJSON, &input) != nil {
			return
		}
		call.Path, summary = input.Path, input.Path
	case builtin.Edit:
		var input fs.EditRequest
		if json.Unmarshal(call.ArgumentsJSON, &input) != nil {
			return
		}
		call.Path, summary = input.Path, input.Path
	case builtin.Grep:
		var input fs.GrepRequest
		if json.Unmarshal(call.ArgumentsJSON, &input) != nil {
			return
		}
		call.Query, call.Path, summary = input.Pattern, input.Path, input.Pattern
	case builtin.Glob:
		var input fs.GlobRequest
		if json.Unmarshal(call.ArgumentsJSON, &input) != nil {
			return
		}
		call.Query, call.Path, summary = input.Pattern, input.Path, input.Pattern
	case builtin.RunScript:
		var input struct {
			Summary string `json:"summary"`
		}
		if json.Unmarshal(call.ArgumentsJSON, &input) != nil {
			return
		}
		summary = input.Summary
	default:
	}
	if summary != "" {
		call.Summary = truncateRunes(summary, toolSummaryRuneLimit)
	}
}

func projectToolResult(call *conversation.ToolCall) error {
	output, err := formattedJSON(call.ResultJSON)
	if err != nil {
		return err
	}
	call.Output = output
	if call.Kind == "" {
		return nil
	}
	switch builtin.Name(call.Name) {
	case builtin.Shell, builtin.ReadShellOutput:
		result, ok := decodeBuiltinResult[builtin.CommandResult](call.ResultJSON)
		if !ok {
			return nil
		}
		call.Output, call.ExitCode = result.Output, result.ExitCode
		call.OutputPage = result.Page
	case builtin.ApplyPatch:
		result, ok := decodeBuiltinResult[builtin.PatchResult](call.ResultJSON)
		if !ok {
			return nil
		}
		paths := make([]string, len(result.Changes))
		for i, change := range result.Changes {
			paths[i] = change.Path
		}
		if len(paths) != 0 {
			call.Path, call.Output = paths[0], strings.Join(paths, "\n")
		}
	case builtin.RunScript:
		result, ok := decodeBuiltinResult[builtin.ScriptResult](call.ResultJSON)
		if !ok {
			return nil
		}
		call.Output = scriptOutput(result)
	default:
	}
	return nil
}

// A builtin result that does not match its typed shape keeps the formatted raw JSON as its presentation.
func scriptOutput(result builtin.ScriptResult) string {
	lines := []string{result.Output}
	if result.Error != "" {
		lines = append(lines, result.Error)
	}
	for _, call := range result.Calls {
		line := call.Tool + " · " + string(call.Status)
		if call.Error != "" {
			line += ": " + call.Error
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func decodeBuiltinResult[T any](encoded []byte) (T, bool) {
	var result T
	return result, json.Unmarshal(encoded, &result) == nil
}

func formattedJSON(encoded []byte) (string, error) {
	formatted := jsontext.Value(bytes.Clone(encoded))
	if err := formatted.Indent(jsontext.WithIndent("  ")); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(formatted)), nil
}
