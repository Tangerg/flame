package terminal

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func (a *app) openApprovalArgumentEditor() {
	if a.dialogs.approval == nil {
		return
	}
	a.dialogs.approvalEditor = a.openContextEditor(contextEditorRequest{
		Title:       "Edit tool arguments",
		Description: "The replacement applies once and is validated as one non-empty JSON object.",
		Content:     a.dialogs.approvalArguments,
		Placeholder: "{\n  \"argument\": \"replacement\"\n}",
		Save: func(value string, complete func(error) bool) error {
			override, err := agent.ParseToolArgumentOverride([]byte(value))
			if err != nil {
				complete(err)
				return nil
			}
			if complete(nil) {
				a.dialogs.approvalEditor = nil
				a.dialogs.approvalOverride = override
				a.dialogs.approvalArguments = formatToolArguments(override.JSON())
				a.setApprovalPreview(a.approvalPreviewSections())
				a.setApprovalForm(approvalAllowOnce)
				a.dialogs.approvalPane.Focus(true)
				a.dialogs.approvalDialog.Controller().SetDescription("Arguments edited · choose how to proceed")
			}
			return nil
		},
		Dismissed: func() { a.dialogs.approvalEditor = nil },
	})
}

func (a *app) approvalPreviewSections() []ToolSection {
	sections := slices.Clone(a.dialogs.approvalSections)
	if a.dialogs.approvalOverride == nil {
		return sections
	}
	return append(sections, ToolSection{
		Title: "Edited arguments · one-shot", Style: toolSectionCode,
		Language: "json", Text: a.dialogs.approvalArguments,
	})
}

func (a *app) dismissApprovalEditor() {
	if a.dialogs.approvalEditor == nil {
		return
	}
	a.dialogs.approvalEditor.Dismiss()
	a.dialogs.approvalEditor = nil
}

func editableApprovalArguments(call *agent.ToolCall) string {
	if call == nil || len(call.ArgumentsJSON) == 0 {
		return "{}"
	}
	return formatToolArguments(call.ArgumentsJSON)
}

func formatToolArguments(encoded []byte) string {
	var formatted bytes.Buffer
	if json.Indent(&formatted, encoded, "", "  ") == nil {
		return formatted.String()
	}
	return string(encoded)
}
