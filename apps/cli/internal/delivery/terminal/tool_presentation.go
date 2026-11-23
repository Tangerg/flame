package terminal

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/failure"
)

const (
	contentSectionTitle = "Content"
	outputSectionTitle  = "Output"
	genericToolLabel    = "tool"
)

// Indent reformats in place and may spend the caller's spare capacity, so the
// projection is cloned first.
func prettyJSON(encoded []byte) string {
	formatted := jsontext.Value(bytes.Clone(encoded))
	if err := formatted.Indent(jsontext.WithIndent("  ")); err != nil {
		return string(encoded)
	}
	return string(formatted)
}

const (
	argumentsSection = "Arguments"
	resultSection    = "Result"
)

const (
	codeLanguageJSON  = "json"
	codeLanguageDiff  = "diff"
	codeLanguagePlain = "text"
)

func jsonSection(title, text string) toolSection {
	return toolSection{Title: title, Style: toolSectionCode, Language: codeLanguageJSON, Text: text}
}

type toolSectionStyle uint8

const (
	toolSectionCode toolSectionStyle = iota
	toolSectionDiffRows
	toolSectionParagraph
)

type toolSection struct {
	Title       string
	Style       toolSectionStyle
	Language    string
	Text        string
	Rows        []protocol.DiffRow
	LineNumbers bool
	Links       bool
}

type toolPresentation struct {
	Label    string
	Sections []toolSection
}

type toolPresenter struct {
	ID      string
	Matches func(conversation.ToolCall) bool
	Present func(conversation.ToolCall) toolPresentation
}

func defaultToolPresenters() []toolPresenter {
	return []toolPresenter{
		kindToolPresenter(builtin.CategoryShell, presentShellTool),
		kindToolPresenter(builtin.CategoryEdit, presentEditTool),
		kindToolPresenter(builtin.CategoryRead, presentReadTool),
		kindToolPresenter(builtin.CategorySearch, presentSearchTool),
		kindToolPresenter(builtin.CategoryTask, presentTaskTool),
		{ID: "generic", Matches: func(conversation.ToolCall) bool { return true }, Present: presentUnknownTool},
	}
}

func kindToolPresenter(kind builtin.ToolCategory, present func(conversation.ToolCall) toolPresentation) toolPresenter {
	return toolPresenter{
		ID: string(kind),
		Matches: func(call conversation.ToolCall) bool {
			return call.Kind == kind
		},
		Present: present,
	}
}

func presentShellTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: shellToolLabel(call),
		Sections: toolSections(call, toolSection{
			Title: outputSectionTitle, Style: toolSectionCode, Text: call.Output,
		}),
	}
}

func presentEditTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: toolKindLabel("edit", toolPrimary(call.Path, call.Summary)),
		Sections: toolSections(call, toolSection{
			Title: outputSectionTitle, Style: toolSectionCode, Text: call.Output,
		}),
	}
}

func presentReadTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: toolKindLabel("read", toolPrimary(call.Path, call.Summary)),
		Sections: toolSections(call, toolSection{
			Title: contentSectionTitle, Style: toolSectionCode, Language: languageForPath(call.Path), Text: call.Output, LineNumbers: true,
		}),
	}
}

func presentSearchTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: toolKindLabel("search", toolPrimary(call.Query, call.Summary)),
		Sections: toolSections(call, toolSection{
			Title: "Matches", Style: toolSectionParagraph, Text: call.Output,
		}),
	}
}

func presentTaskTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: toolKindLabel("task", strings.TrimSpace(call.Summary)),
		Sections: toolSections(call, toolSection{
			Title: resultSection, Style: toolSectionParagraph, Text: call.Output,
		}),
	}
}

func presentUnknownTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: unknownToolLabel(call),
		Sections: toolSections(call, toolSection{
			Title: outputSectionTitle, Style: toolSectionCode, Text: call.Output,
		}),
	}
}

func executionMetadata(call conversation.ToolCall) []string {
	var metadata []string
	if call.Safety != "" {
		metadata = append(metadata, "safety   "+string(call.Safety))
	}
	if !call.StartedAt.IsZero() {
		metadata = append(metadata, "started  "+call.StartedAt.Format(time.RFC3339))
	}
	if !call.FinishedAt.IsZero() {
		metadata = append(metadata, "finished "+call.FinishedAt.Format(time.RFC3339))
	}
	if page := call.OutputPage; page != nil {
		metadata = append(metadata, outputPageMetadata(*page)...)
	}
	return metadata
}

func outputPageMetadata(page builtin.CommandOutputPage) []string {
	metadata := []string{fmt.Sprintf("output bytes [%d, %d)", page.Offset, page.NextOffset)}
	if page.ShellID != "" {
		metadata = append(metadata, "shell    "+page.ShellID)
	}
	if page.Status != "" {
		metadata = append(metadata, "process  "+page.Status)
	}
	if page.HasMore {
		metadata = append(metadata, "more output remains in the original shell")
	}
	if page.Dropped {
		metadata = append(metadata, "output recording failed; retained bytes are incomplete")
	}
	return metadata
}

func toolSections(call conversation.ToolCall, output toolSection) []toolSection {
	var sections []toolSection
	if metadata := executionMetadata(call); len(metadata) > 0 {
		sections = append(sections, toolSection{
			Title: "Execution", Style: toolSectionCode, Text: strings.Join(metadata, "\n"),
		})
	}
	if call.ArgumentsText != "" {
		sections = append(sections, toolSection{Title: argumentsSection, Style: toolSectionCode, Text: call.ArgumentsText})
	} else if len(call.ArgumentsJSON) != 0 {
		sections = append(sections, jsonSection(argumentsSection, prettyJSON(call.ArgumentsJSON)))
	}
	if strings.TrimSpace(output.Text) != "" {
		sections = append(sections, output)
	}
	if len(call.ResultJSON) != 0 {
		sections = append(sections, jsonSection(resultSection, prettyJSON(call.ResultJSON)))
	}
	if call.Problem != nil {
		encoded, err := json.Marshal(call.Problem, json.Deterministic(true))
		if err != nil {
			encoded = []byte(failure.String(call.Problem))
		}
		sections = append(sections, jsonSection("Problem", prettyJSON(encoded)))
	}
	return sections
}

func selectToolPresentation(presenters []toolPresenter, call conversation.ToolCall) (toolPresentation, error) {
	if len(presenters) == 0 {
		presenters = defaultToolPresenters()
	} else {
		presenters = slices.Clone(presenters)
	}
	for _, presenter := range presenters {
		if err := validateToolPresenter(presenter); err != nil {
			return toolPresentation{}, err
		}
		matches, err := matchToolSafely(presenter, call)
		if err != nil {
			return toolPresentation{}, err
		}
		if !matches {
			continue
		}
		presentation, err := presentToolSafely(presenter, call)
		if err != nil {
			return toolPresentation{}, err
		}
		if strings.TrimSpace(presentation.Label) == "" {
			return toolPresentation{}, fmt.Errorf("tool presenter %q returned an empty label", presenter.ID)
		}
		presentation.Sections = slices.Clone(presentation.Sections)
		return presentation, nil
	}
	return toolPresentation{}, errors.New("no tool presenter matched the call")
}

func validateToolPresenter(presenter toolPresenter) error {
	switch {
	case strings.TrimSpace(presenter.ID) == "":
		return errors.New("tool presenter has no id")
	case presenter.Matches == nil:
		return fmt.Errorf("tool presenter %q has no matcher", presenter.ID)
	case presenter.Present == nil:
		return fmt.Errorf("tool presenter %q has no projection", presenter.ID)
	default:
		return nil
	}
}

func matchToolSafely(presenter toolPresenter, call conversation.ToolCall) (matched bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tool presenter %q matcher panicked: %v", presenter.ID, recovered)
		}
	}()
	return presenter.Matches(call), nil
}

func presentToolSafely(presenter toolPresenter, call conversation.ToolCall) (presentation toolPresentation, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tool presenter %q projection panicked: %v", presenter.ID, recovered)
		}
	}()
	return presenter.Present(call), nil
}

func toolLabel(call conversation.ToolCall) string {
	presentation, err := selectToolPresentation(nil, call)
	if err != nil {
		return unknownToolLabel(call)
	}
	return presentation.Label
}

func shellToolLabel(call conversation.ToolCall) string {
	primary := toolPrimary(call.Command, call.Summary)
	if primary == "" {
		return "shell"
	}
	return "$ " + primary
}

func unknownToolLabel(call conversation.ToolCall) string {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		name = genericToolLabel
	}
	return toolKindLabel(name, strings.TrimSpace(call.Summary))
}

func toolPrimary(specific, fallback string) string {
	if specific = strings.TrimSpace(specific); specific != "" {
		return specific
	}
	return strings.TrimSpace(fallback)
}

func toolKindLabel(kind, primary string) string {
	if primary == "" {
		return kind
	}
	return kind + format.Separator + primary
}
