package terminal

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/failure"
)

// prettyJSON indents a value the reader inspects as JSON. Unreadable bytes are
// shown verbatim: a panel that hides a malformed payload hides the problem.
// Indent reformats in place and will spend the caller's spare capacity, so the
// projection it is handed is cloned rather than rewritten.
func prettyJSON(encoded []byte) string {
	formatted := jsontext.Value(bytes.Clone(encoded))
	if err := formatted.Indent(jsontext.WithIndent("  ")); err != nil {
		return string(encoded)
	}
	return string(formatted)
}

type toolSectionStyle uint8

const (
	toolSectionCode toolSectionStyle = iota
	toolSectionDiff
	toolSectionParagraph
)

// toolSection is one semantic region of a tool presentation. It deliberately
// carries no Oolong component so inline blocks, the reader, and approvals can
// project the same meaning at different sizes without sharing widget state.
type toolSection struct {
	Title       string
	Style       toolSectionStyle
	Language    string
	Text        string
	LineNumbers bool
	Links       bool
}

// toolPresentation is the deterministic terminal meaning of one tool call.
type toolPresentation struct {
	Label    string
	Sections []toolSection
}

// toolPresenter claims tool calls it understands and produces their semantic
// presentation. Matchers are ordered; the first match wins and the built-in
// generic presenter remains the final fallback.
type toolPresenter struct {
	ID      string
	Matches func(conversation.ToolCall) bool
	Present func(conversation.ToolCall) toolPresentation
}

func defaultToolPresenters() []toolPresenter {
	return []toolPresenter{
		kindToolPresenter(protocol.ToolCategoryShell, presentShellTool),
		kindToolPresenter(protocol.ToolCategoryEdit, presentEditTool),
		kindToolPresenter(protocol.ToolCategoryRead, presentReadTool),
		kindToolPresenter(protocol.ToolCategorySearch, presentSearchTool),
		kindToolPresenter(protocol.ToolCategoryWeb, presentWebTool),
		kindToolPresenter(protocol.ToolCategoryTask, presentTaskTool),
		{ID: "generic", Matches: func(conversation.ToolCall) bool { return true }, Present: presentUnknownTool},
	}
}

func kindToolPresenter(kind protocol.ToolCategory, present func(conversation.ToolCall) toolPresentation) toolPresenter {
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
			Title: "Output", Style: toolSectionCode, Text: call.Output,
		}),
	}
}

func presentEditTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: toolKindLabel("edit", toolPrimary(call.Path, call.Summary)),
		Sections: toolSections(call, toolSection{
			Title: "Output", Style: toolSectionCode, Text: call.Output,
		}),
	}
}

func presentReadTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: toolKindLabel("read", toolPrimary(call.Path, call.Summary)),
		Sections: toolSections(call, toolSection{
			Title: "Content", Style: toolSectionCode, Language: languageForPath(call.Path), Text: call.Output, LineNumbers: true,
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

func presentWebTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: toolKindLabel("web", toolPrimary(call.URL, call.Summary)),
		Sections: toolSections(call, toolSection{
			Title: "Response", Style: toolSectionParagraph, Text: call.Output, Links: true,
		}),
	}
}

func presentTaskTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: toolKindLabel("task", strings.TrimSpace(call.Summary)),
		Sections: toolSections(call, toolSection{
			Title: "Result", Style: toolSectionParagraph, Text: call.Output,
		}),
	}
}

func presentUnknownTool(call conversation.ToolCall) toolPresentation {
	return toolPresentation{
		Label: unknownToolLabel(call),
		Sections: toolSections(call, toolSection{
			Title: "Output", Style: toolSectionCode, Text: call.Output,
		}),
	}
}

func toolSections(call conversation.ToolCall, output toolSection) []toolSection {
	sections := make([]toolSection, 0, 6)
	metadata := make([]string, 0, 3)
	if call.Safety != "" {
		metadata = append(metadata, "safety   "+string(call.Safety))
	}
	if !call.StartedAt.IsZero() {
		metadata = append(metadata, "started  "+call.StartedAt.Format(time.RFC3339))
	}
	if !call.FinishedAt.IsZero() {
		metadata = append(metadata, "finished "+call.FinishedAt.Format(time.RFC3339))
	}
	if len(metadata) > 0 {
		sections = append(sections, toolSection{
			Title: "Execution", Style: toolSectionCode, Text: strings.Join(metadata, "\n"),
		})
	}
	if call.ArgumentsText != "" {
		sections = append(sections, toolSection{Title: "Arguments", Style: toolSectionCode, Text: call.ArgumentsText})
	} else if len(call.ArgumentsJSON) != 0 {
		sections = append(sections, toolSection{
			Title: "Arguments", Style: toolSectionCode, Language: "json", Text: prettyJSON(call.ArgumentsJSON),
		})
	}
	if strings.TrimSpace(call.Diff) != "" {
		sections = append(sections, toolSection{Title: "Changes", Style: toolSectionDiff, Language: "diff", Text: call.Diff})
	}
	if strings.TrimSpace(output.Text) != "" {
		sections = append(sections, output)
	}
	if len(call.ResultJSON) != 0 {
		sections = append(sections, toolSection{
			Title: "Result", Style: toolSectionCode, Language: "json", Text: prettyJSON(call.ResultJSON),
		})
	}
	if call.Problem != nil {
		encoded, err := json.Marshal(call.Problem, json.Deterministic(true))
		if err != nil {
			encoded = []byte(failure.String(call.Problem))
		}
		sections = append(sections, toolSection{
			Title: "Problem", Style: toolSectionCode, Language: "json", Text: prettyJSON(encoded),
		})
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
		name = "tool"
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
	return kind + " · " + primary
}
