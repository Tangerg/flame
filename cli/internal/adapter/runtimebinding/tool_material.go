package runtimebinding

import (
	"bytes"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

// toolText borrows Runtime's decoded JSON object. Unknown tools may use these
// names for other values, so only string fields contribute to presentation.
func toolText(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := object[key].(string); ok && text != "" {
			return text
		}
	}
	return ""
}

func toolSummary(name string, arguments map[string]any) string {
	text := toolText(arguments, "description", "summary", "query", "pattern", "path", "url", "command")
	if text == "" {
		text = name
	}
	return truncateRunes(text, toolSummaryRuneLimit)
}

func projectToolResult(tool *agent.ToolCall, value any) {
	object, _ := value.(map[string]any)
	tool.Output = toolText(object, "output")
	if exitCode, ok := toolExitCode(object["exitCode"]); ok {
		tool.ExitCode = &exitCode
	}
	changes, _ := object["changes"].([]any)
	var paths []string
	for _, value := range changes {
		change, _ := value.(map[string]any)
		if path := toolText(change, "path"); path != "" {
			paths = append(paths, filepath.ToSlash(path))
		}
	}
	if tool.Path == "" && len(paths) != 0 {
		tool.Path = paths[0]
	}
	if tool.Output == "" && len(paths) != 0 {
		tool.Output = strings.Join(paths, "\n")
	}
	if tool.Output == "" {
		tool.Output = formattedJSON(tool.ResultJSON)
	}
}

// toolExitCode reads the number Runtime decoded. Runtime keeps tool-call numbers
// as written, so a [jsonv1.Number] reaches this binding in process; naming that
// type here is what keeps the exit code an integer. It is the standard library's
// exact-number carrier and encoding/json/v2 has no replacement for it, so the v1
// import is the type alone — this file decodes nothing.
func toolExitCode(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), int64(int(number)) == number
	case float64:
		converted := int(number)
		return converted, float64(converted) == number
	case jsonv1.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil && int64(int(parsed)) == parsed
	default:
		return 0, false
	}
}

// formattedJSON renders a result nobody projected into text. Indent reformats
// in place and will spend the caller's spare capacity, so the tool call's own
// result bytes are cloned rather than rewritten.
func formattedJSON(encoded []byte) string {
	formatted := jsontext.Value(bytes.Clone(encoded))
	if err := formatted.Indent(jsontext.WithIndent("  ")); err != nil {
		return ""
	}
	return strings.TrimSpace(string(formatted))
}
