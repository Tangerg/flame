package terminal

import (
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

func (p *interruptPrompts) openApprovalArgumentEditor() {
	if p.approval == nil {
		return
	}
	p.approvalEditor = p.editContext(contextEditorRequest{
		Title:       "Edit tool arguments",
		Description: "The replacement applies once. Use {} to clear the arguments.",
		Content:     p.approvalArguments,
		Placeholder: "{\n  \"argument\": \"replacement\"\n}",
		Save: func(value string, complete func(error) bool) error {
			override, err := conversation.ParseToolArgumentOverride([]byte(value))
			if err != nil {
				complete(err)
				return nil
			}
			if complete(nil) {
				p.approvalEditor = nil
				p.approvalOverride = override
				p.approvalArguments = prettyJSON(override.JSON())
				p.setApprovalPreview(p.approvalPreviewSections())
				p.setApprovalForm(approvalAllowOnce)
				p.approvalPane.Focus(true)
				p.approvalDialog.Controller().SetDescription("Arguments edited · choose how to proceed")
			}
			return nil
		},
		Dismissed: func() { p.approvalEditor = nil },
	})
}

func (p *interruptPrompts) approvalPreviewSections() []toolSection {
	sections := slices.Clone(p.approvalSections)
	if p.approvalOverride == nil {
		return sections
	}
	return append(sections, toolSection{
		Title: "Edited arguments · one-shot", Style: toolSectionCode,
		Language: "json", Text: p.approvalArguments,
	})
}

func (p *interruptPrompts) dismissApprovalEditor() {
	if p.approvalEditor == nil {
		return
	}
	p.approvalEditor.Dismiss()
	p.approvalEditor = nil
}

func editableApprovalArguments(call *conversation.ToolCall) string {
	if call == nil || len(call.ArgumentsJSON) == 0 {
		return "{}"
	}
	return prettyJSON(call.ArgumentsJSON)
}
