package runtimebinding

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/scope/tools/fs"
)

func projectToolArguments(call *conversation.ToolCall) error {
	if call.Kind == protocol.ToolCategoryUnknown || call.ArgumentsText != "" {
		return nil
	}
	var summary string
	switch builtin.Name(call.Name) {
	case builtin.Shell:
		var input builtin.ShellArguments
		if err := json.Unmarshal(call.ArgumentsJSON, &input); err != nil {
			return err
		}
		call.Command, summary = input.Command, input.Description
	case builtin.Read:
		var input fs.ReadRequest
		if err := json.Unmarshal(call.ArgumentsJSON, &input); err != nil {
			return err
		}
		call.Path, summary = input.Path, input.Path
	case builtin.Edit:
		var input fs.EditRequest
		if err := json.Unmarshal(call.ArgumentsJSON, &input); err != nil {
			return err
		}
		call.Path, summary = input.Path, input.Path
	case builtin.Grep:
		var input fs.GrepRequest
		if err := json.Unmarshal(call.ArgumentsJSON, &input); err != nil {
			return err
		}
		call.Query, call.Path, summary = input.Pattern, input.Path, input.Pattern
	case builtin.Glob:
		var input fs.GlobRequest
		if err := json.Unmarshal(call.ArgumentsJSON, &input); err != nil {
			return err
		}
		call.Query, call.Path, summary = input.Pattern, input.Path, input.Pattern

	}
	if summary != "" {
		call.Summary = truncateRunes(summary, toolSummaryRuneLimit)
	}
	return nil
}

func projectToolResult(call *conversation.ToolCall) error {
	output, err := formattedJSON(call.ResultJSON)
	if err != nil {
		return err
	}
	call.Output = output
	if call.Kind == protocol.ToolCategoryUnknown {
		return nil
	}
	switch builtin.Name(call.Name) {
	case builtin.Shell:
		var result builtin.CommandResult
		if err := json.Unmarshal(call.ResultJSON, &result); err != nil {
			return err
		}
		call.Output, call.ExitCode = result.Output, result.ExitCode
	case builtin.ApplyPatch:
		var result builtin.PatchResult
		if err := json.Unmarshal(call.ResultJSON, &result); err != nil {
			return err
		}
		paths := make([]string, len(result.Changes))
		for i, change := range result.Changes {
			paths[i] = change.Path
		}
		if len(paths) != 0 {
			call.Path, call.Output = paths[0], strings.Join(paths, "\n")
		}
	}
	return nil
}

// JSON material remains exact and complete when no declared projection owns it.
func formattedJSON(encoded []byte) (string, error) {
	formatted := jsontext.Value(bytes.Clone(encoded))
	if err := formatted.Indent(jsontext.WithIndent("  ")); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(formatted)), nil
}
