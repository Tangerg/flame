package runtimebinding

import (
	"bytes"
	"encoding/json"
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

func toolExitCode(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), int64(int(number)) == number
	case float64:
		converted := int(number)
		return converted, float64(converted) == number
	case json.Number:
		parsed, err := number.Int64()
		return int(parsed), err == nil && int64(int(parsed)) == parsed
	default:
		return 0, false
	}
}

func formattedJSON(encoded []byte) string {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, encoded, "", "  "); err != nil {
		return ""
	}
	return strings.TrimSpace(formatted.String())
}
