package runtimeembedded

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
)

// toolArgumentsMaterial is the small presentation projection understood by
// the CLI. The complete open-ended argument object remains in ArgumentsJSON;
// this type prevents wire maps and their member names leaking into consumers.
type toolArgumentsMaterial struct {
	Description json.RawMessage `json:"description"`
	Summary     json.RawMessage `json:"summary"`
	Query       json.RawMessage `json:"query"`
	Pattern     json.RawMessage `json:"pattern"`
	Search      json.RawMessage `json:"search"`
	Path        json.RawMessage `json:"path"`
	File        json.RawMessage `json:"file"`
	Filename    json.RawMessage `json:"filename"`
	URL         json.RawMessage `json:"url"`
	URI         json.RawMessage `json:"uri"`
	Command     json.RawMessage `json:"command"`
}

func decodeToolArgumentsMaterial(encoded []byte) toolArgumentsMaterial {
	var material toolArgumentsMaterial
	_ = json.Unmarshal(encoded, &material)
	return material
}

func (m toolArgumentsMaterial) command() string { return decodedString(m.Command) }

func (m toolArgumentsMaterial) path() string {
	return firstDecodedString(m.Path, m.File, m.Filename)
}

func (m toolArgumentsMaterial) query() string {
	return firstDecodedString(m.Query, m.Pattern, m.Search)
}

func (m toolArgumentsMaterial) url() string {
	return firstDecodedString(m.URL, m.URI)
}

func (m toolArgumentsMaterial) summary(toolName string) string {
	for _, value := range []json.RawMessage{
		m.Description, m.Summary, m.Query, m.Pattern, m.Path, m.URL, m.Command,
	} {
		if text := decodedString(value); text != "" {
			return truncateRunes(text, toolSummaryRuneLimit)
		}
	}
	return truncateRunes(toolName, toolSummaryRuneLimit)
}

type toolResultMaterial struct {
	Output   json.RawMessage `json:"output"`
	ExitCode json.RawMessage `json:"exitCode"`
	Changes  json.RawMessage `json:"changes"`
}

type toolChangeMaterial struct {
	Path json.RawMessage `json:"path"`
}

func decodeToolResultMaterial(encoded []byte) toolResultMaterial {
	var material toolResultMaterial
	_ = json.Unmarshal(encoded, &material)
	return material
}

func (m toolResultMaterial) output() string { return decodedString(m.Output) }

func (m toolResultMaterial) exitCode() (int, bool) {
	if len(m.ExitCode) == 0 {
		return 0, false
	}
	decoder := json.NewDecoder(bytes.NewReader(m.ExitCode))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return 0, false
	}
	return integerValue(value)
}

func (m toolResultMaterial) changedPaths() []string {
	var changes []json.RawMessage
	if err := json.Unmarshal(m.Changes, &changes); err != nil {
		return nil
	}
	paths := make([]string, 0, len(changes))
	for _, encoded := range changes {
		var change toolChangeMaterial
		if err := json.Unmarshal(encoded, &change); err != nil {
			continue
		}
		if path := decodedString(change.Path); path != "" {
			paths = append(paths, filepath.ToSlash(path))
		}
	}
	return paths
}

func decodedString(encoded json.RawMessage) string {
	var value string
	if err := json.Unmarshal(encoded, &value); err != nil {
		return ""
	}
	return value
}

func firstDecodedString(values ...json.RawMessage) string {
	for _, value := range values {
		if decoded := decodedString(value); decoded != "" {
			return decoded
		}
	}
	return ""
}

func formattedJSON(encoded []byte) string {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, encoded, "", "  "); err != nil {
		return ""
	}
	return strings.TrimSpace(formatted.String())
}
