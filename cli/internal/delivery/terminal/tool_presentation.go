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

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/cli/internal/domain/failure"
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

// ToolSection is one semantic region of a tool presentation. It deliberately
// carries no Oolong component so inline blocks, the reader, and approvals can
// project the same meaning at different sizes without sharing widget state.
type ToolSection struct {
	Title       string
	Style       toolSectionStyle
	Language    string
	Text        string
	LineNumbers bool
	Links       bool
}

// ToolPresentation is the deterministic terminal meaning of one tool call.
type ToolPresentation struct {
	Label    string
	Sections []ToolSection
}

// ToolPresenter claims tool calls it understands and produces their semantic
// presentation. Matchers are ordered; the first match wins and the built-in
// generic presenter remains the final fallback.
type ToolPresenter struct {
	ID      string
	Matches func(conversation.ToolCall) bool
	Present func(conversation.ToolCall) ToolPresentation
}

func defaultToolPresenters() []ToolPresenter {
	return []ToolPresenter{
		kindToolPresenter("shell", conversation.ToolShell, presentShellTool),
		kindToolPresenter("edit", conversation.ToolEdit, presentEditTool),
		kindToolPresenter("read", conversation.ToolRead, presentReadTool),
		kindToolPresenter("search", conversation.ToolSearch, presentSearchTool),
		kindToolPresenter("web", conversation.ToolWeb, presentWebTool),
		kindToolPresenter("task", conversation.ToolTask, presentTaskTool),
		{ID: "generic", Matches: func(conversation.ToolCall) bool { return true }, Present: presentUnknownTool},
	}
}

func kindToolPresenter(id string, kind conversation.ToolKind, present func(conversation.ToolCall) ToolPresentation) ToolPresenter {
	return ToolPresenter{
		ID: id,
		Matches: func(call conversation.ToolCall) bool {
			return call.Kind == kind
		},
		Present: present,
	}
}

func presentShellTool(call conversation.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: shellToolLabel(call),
		Sections: toolSections(call, ToolSection{
			Title: "Output", Style: toolSectionCode, Text: call.Output,
		}),
	}
}

func presentEditTool(call conversation.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: toolKindLabel("edit", toolPrimary(call.Path, call.Summary)),
		Sections: toolSections(call, ToolSection{
			Title: "Output", Style: toolSectionCode, Text: call.Output,
		}),
	}
}

func presentReadTool(call conversation.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: toolKindLabel("read", toolPrimary(call.Path, call.Summary)),
		Sections: toolSections(call, ToolSection{
			Title: "Content", Style: toolSectionCode, Language: languageForPath(call.Path), Text: call.Output, LineNumbers: true,
		}),
	}
}

func presentSearchTool(call conversation.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: toolKindLabel("search", toolPrimary(call.Query, call.Summary)),
		Sections: toolSections(call, ToolSection{
			Title: "Matches", Style: toolSectionParagraph, Text: call.Output,
		}),
	}
}

func presentWebTool(call conversation.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: toolKindLabel("web", toolPrimary(call.URL, call.Summary)),
		Sections: toolSections(call, ToolSection{
			Title: "Response", Style: toolSectionParagraph, Text: call.Output, Links: true,
		}),
	}
}

func presentTaskTool(call conversation.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: toolKindLabel("task", strings.TrimSpace(call.Summary)),
		Sections: toolSections(call, ToolSection{
			Title: "Result", Style: toolSectionParagraph, Text: call.Output,
		}),
	}
}

func presentUnknownTool(call conversation.ToolCall) ToolPresentation {
	return ToolPresentation{
		Label: unknownToolLabel(call),
		Sections: toolSections(call, ToolSection{
			Title: "Output", Style: toolSectionCode, Text: call.Output,
		}),
	}
}

func toolSections(call conversation.ToolCall, output ToolSection) []ToolSection {
	sections := make([]ToolSection, 0, 6)
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
		sections = append(sections, ToolSection{
			Title: "Execution", Style: toolSectionCode, Text: strings.Join(metadata, "\n"),
		})
	}
	if call.ArgumentsText != "" {
		sections = append(sections, ToolSection{Title: "Arguments", Style: toolSectionCode, Text: call.ArgumentsText})
	} else if len(call.ArgumentsJSON) != 0 {
		sections = append(sections, ToolSection{
			Title: "Arguments", Style: toolSectionCode, Language: "json", Text: prettyJSON(call.ArgumentsJSON),
		})
	}
	if strings.TrimSpace(call.Diff) != "" {
		sections = append(sections, ToolSection{Title: "Changes", Style: toolSectionDiff, Language: "diff", Text: call.Diff})
	}
	if strings.TrimSpace(output.Text) != "" {
		sections = append(sections, output)
	}
	if len(call.ResultJSON) != 0 {
		sections = append(sections, ToolSection{
			Title: "Result", Style: toolSectionCode, Language: "json", Text: prettyJSON(call.ResultJSON),
		})
	}
	if call.Problem != nil {
		encoded, err := json.Marshal(call.Problem, json.Deterministic(true))
		if err != nil {
			encoded = []byte(failure.String(call.Problem))
		}
		sections = append(sections, ToolSection{
			Title: "Problem", Style: toolSectionCode, Language: "json", Text: prettyJSON(encoded),
		})
	}
	return sections
}

func selectToolPresentation(presenters []ToolPresenter, call conversation.ToolCall) (ToolPresentation, error) {
	if len(presenters) == 0 {
		presenters = defaultToolPresenters()
	} else {
		presenters = slices.Clone(presenters)
	}
	for _, presenter := range presenters {
		if err := validateToolPresenter(presenter); err != nil {
			return ToolPresentation{}, err
		}
		matches, err := matchToolSafely(presenter, call)
		if err != nil {
			return ToolPresentation{}, err
		}
		if !matches {
			continue
		}
		presentation, err := presentToolSafely(presenter, call)
		if err != nil {
			return ToolPresentation{}, err
		}
		if strings.TrimSpace(presentation.Label) == "" {
			return ToolPresentation{}, fmt.Errorf("tool presenter %q returned an empty label", presenter.ID)
		}
		presentation.Sections = slices.Clone(presentation.Sections)
		return presentation, nil
	}
	return ToolPresentation{}, errors.New("no tool presenter matched the call")
}

func validateToolPresenter(presenter ToolPresenter) error {
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

func matchToolSafely(presenter ToolPresenter, call conversation.ToolCall) (matched bool, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("tool presenter %q matcher panicked: %v", presenter.ID, recovered)
		}
	}()
	return presenter.Matches(call), nil
}

func presentToolSafely(presenter ToolPresenter, call conversation.ToolCall) (presentation ToolPresentation, err error) {
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
