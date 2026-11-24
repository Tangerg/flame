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

// argumentSummary is what a builtin's arguments contribute to its transcript row.
type argumentSummary struct {
	command, path, query, text string
}

func summarizeToolArguments(call *conversation.ToolCall) {
	if call.Kind == "" || call.ArgumentsText != "" {
		return
	}
	summary, ok := builtinArgumentSummary(builtin.Name(call.Name), call.ArgumentsJSON)
	if !ok {
		return
	}
	call.Command, call.Path, call.Query = summary.command, summary.path, summary.query
	if summary.text != "" {
		call.Summary = truncateRunes(summary.text, toolSummaryRuneLimit)
	}
}

func builtinArgumentSummary(name builtin.Name, encoded []byte) (argumentSummary, bool) {
	switch name {
	case builtin.Shell:
		return summarized(encoded, func(input builtin.ShellArguments) argumentSummary {
			return argumentSummary{command: input.Command, text: input.Description}
		})
	case builtin.Read:
		return summarized(encoded, func(input fs.ReadRequest) argumentSummary {
			return argumentSummary{path: input.Path, text: input.Path}
		})
	case builtin.Edit:
		return summarized(encoded, func(input fs.EditRequest) argumentSummary {
			return argumentSummary{path: input.Path, text: input.Path}
		})
	case builtin.Grep:
		return summarized(encoded, func(input fs.GrepRequest) argumentSummary {
			return argumentSummary{query: input.Pattern, path: input.Path, text: input.Pattern}
		})
	case builtin.Glob:
		return summarized(encoded, func(input fs.GlobRequest) argumentSummary {
			return argumentSummary{query: input.Pattern, path: input.Path, text: input.Pattern}
		})
	case builtin.RunScript:
		return summarized(encoded, func(input builtin.ScriptArguments) argumentSummary {
			return argumentSummary{text: input.Summary}
		})
	default:
		return argumentSummary{}, false
	}
}

func summarized[T any](encoded []byte, summarize func(T) argumentSummary) (argumentSummary, bool) {
	input, ok := decodeBuiltin[T](encoded)
	if !ok {
		return argumentSummary{}, false
	}
	return summarize(input), true
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
		result, ok := decodeBuiltin[builtin.CommandResult](call.ResultJSON)
		if !ok {
			return nil
		}
		call.Output, call.ExitCode = result.Output, result.ExitCode
		call.OutputPage = result.Page
	case builtin.ApplyPatch:
		result, ok := decodeBuiltin[builtin.PatchResult](call.ResultJSON)
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
		result, ok := decodeBuiltin[builtin.ScriptResult](call.ResultJSON)
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
	if result.Status != builtin.ScriptCompleted {
		lines = append(lines, string(result.Status))
	}
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

func decodeBuiltin[T any](encoded []byte) (T, bool) {
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
