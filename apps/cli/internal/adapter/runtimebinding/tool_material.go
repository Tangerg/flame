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

// summarizeToolArguments derives an optional typed summary for a built-in tool.
// The Runtime already admitted the Item, so material that does not fit the
// executable shape (a failed call's observed arguments) keeps its complete
// ArgumentsJSON and simply has no typed summary.
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
	}
	if summary != "" {
		call.Summary = truncateRunes(summary, toolSummaryRuneLimit)
	}
}

// projectToolResult always keeps the complete formatted result; a built-in
// result that does not fit its typed shape only lacks the typed fields.
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
	case builtin.Shell:
		var result builtin.CommandResult
		if json.Unmarshal(call.ResultJSON, &result) != nil {
			return nil
		}
		call.Output, call.ExitCode = result.Output, result.ExitCode
	case builtin.ApplyPatch:
		var result builtin.PatchResult
		if json.Unmarshal(call.ResultJSON, &result) != nil {
			return nil
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
