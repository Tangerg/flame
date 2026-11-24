package terminal

import (
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/base"
)

type approvalAction string

const (
	approvalAllowOnce    approvalAction = "allow-once"
	approvalAllowSession approvalAction = "allow-session"
	approvalAllowWorkspace approvalAction = "allow-workspace"
	approvalAllowGlobal  approvalAction = "allow-global"
	approvalDenyOnce     approvalAction = "deny-once"
	approvalDenySession  approvalAction = "deny-session"
	approvalDenyWorkspace  approvalAction = "deny-workspace"
	approvalDenyGlobal   approvalAction = "deny-global"
	approvalEditArgs     approvalAction = "edit-arguments"
)

func approvalOptions(rememberable bool) []base.Option[approvalAction] {
	options := []base.Option[approvalAction]{
		{Label: "Allow once", Value: approvalAllowOnce},
		{Label: "Deny once", Value: approvalDenyOnce},
		{Label: "Edit arguments before deciding", Value: approvalEditArgs},
	}
	if !rememberable {
		return options
	}
	options = slices.Insert(options, 1,
		base.Option[approvalAction]{Label: "Allow for this session", Value: approvalAllowSession},
		base.Option[approvalAction]{Label: "Allow in this workspace", Value: approvalAllowWorkspace},
		base.Option[approvalAction]{Label: "Always allow this rule", Value: approvalAllowGlobal},
	)
	return slices.Insert(options, len(options)-1,
		base.Option[approvalAction]{Label: "Deny for this session", Value: approvalDenySession},
		base.Option[approvalAction]{Label: "Deny in this workspace", Value: approvalDenyWorkspace},
		base.Option[approvalAction]{Label: "Always deny this rule", Value: approvalDenyGlobal},
	)
}

func (a approvalAction) Normalize(rememberable bool) (approvalAction, bool) {
	for _, option := range approvalOptions(rememberable) {
		if option.Value == a {
			return a, true
		}
	}
	answer, ok := a.Answer()
	if !ok {
		return "", false
	}
	if answer.Decision == protocol.ApprovalDeny {
		return approvalDenyOnce, true
	}
	return approvalAllowOnce, true
}

func defaultApprovalAction(preference settings.RememberPreference) approvalAction {
	switch preference {
	case settings.RememberSession:
		return approvalAllowSession
	case settings.RememberWorkspace:
		return approvalAllowWorkspace
	case settings.RememberGlobal:
		return approvalAllowGlobal
	default:
		return approvalAllowOnce
	}
}

func (a approvalAction) Answer() (conversation.ApprovalAnswer, bool) {
	switch a {
	case approvalAllowSession:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove, Remember: protocol.RememberSession}, true
	case approvalAllowWorkspace:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove, Remember: protocol.RememberWorkspace}, true
	case approvalAllowGlobal:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove, Remember: protocol.RememberGlobal}, true
	case approvalAllowOnce:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove}, true
	case approvalDenySession:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny, Remember: protocol.RememberSession}, true
	case approvalDenyWorkspace:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny, Remember: protocol.RememberWorkspace}, true
	case approvalDenyGlobal:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny, Remember: protocol.RememberGlobal}, true
	case approvalDenyOnce:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny}, true
	default:
		return conversation.ApprovalAnswer{}, false
	}
}

func approvalActionFromAnswer(answer conversation.ApprovalAnswer) approvalAction {
	for _, option := range approvalOptions(true) {
		candidate, ok := option.Value.Answer()
		if ok && candidate.Decision == answer.Decision && candidate.Remember == answer.Remember {
			return option.Value
		}
	}
	return ""
}
