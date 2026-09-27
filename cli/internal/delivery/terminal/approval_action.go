package terminal

import (
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/components/headless"
)

// approvalAction is the terminal's complete decision vocabulary for one tool
// approval. Keeping it closed prevents form choices and runtime answers from
// drifting as new persistence scopes are exposed.
type approvalAction string

const (
	approvalAllowOnce    approvalAction = "allow-once"
	approvalAllowSession approvalAction = "allow-session"
	approvalAllowProject approvalAction = "allow-project"
	approvalAllowGlobal  approvalAction = "allow-global"
	approvalDenyOnce     approvalAction = "deny-once"
	approvalDenySession  approvalAction = "deny-session"
	approvalDenyProject  approvalAction = "deny-project"
	approvalDenyGlobal   approvalAction = "deny-global"
	approvalEditArgs     approvalAction = "edit-arguments"
)

func approvalOptions(rememberable bool) []headless.Option[approvalAction] {
	options := []headless.Option[approvalAction]{
		{Label: "Allow once", Value: approvalAllowOnce},
		{Label: "Deny once", Value: approvalDenyOnce},
		{Label: "Edit arguments before deciding", Value: approvalEditArgs},
	}
	if !rememberable {
		return options
	}
	options = slices.Insert(options, 1,
		headless.Option[approvalAction]{Label: "Allow for this session", Value: approvalAllowSession},
		headless.Option[approvalAction]{Label: "Allow for this project", Value: approvalAllowProject},
		headless.Option[approvalAction]{Label: "Always allow this rule", Value: approvalAllowGlobal},
	)
	return slices.Insert(options, len(options)-1,
		headless.Option[approvalAction]{Label: "Deny for this session", Value: approvalDenySession},
		headless.Option[approvalAction]{Label: "Deny for this project", Value: approvalDenyProject},
		headless.Option[approvalAction]{Label: "Always deny this rule", Value: approvalDenyGlobal},
	)
}

func (a approvalAction) Normalize(rememberable bool) approvalAction {
	for _, option := range approvalOptions(rememberable) {
		if option.Value == a {
			return a
		}
	}
	return approvalAllowOnce
}

func defaultApprovalAction(scope protocol.RememberScopeKind) approvalAction {
	switch scope {
	case protocol.RememberSession:
		return approvalAllowSession
	case protocol.RememberProject:
		return approvalAllowProject
	case protocol.RememberGlobal:
		return approvalAllowGlobal
	default:
		return approvalAllowOnce
	}
}

func (a approvalAction) Answer() (conversation.ApprovalAnswer, bool) {
	switch a {
	case approvalAllowSession:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove, Remember: protocol.RememberSession}, true
	case approvalAllowProject:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove, Remember: protocol.RememberProject}, true
	case approvalAllowGlobal:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove, Remember: protocol.RememberGlobal}, true
	case approvalAllowOnce:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove}, true
	case approvalDenySession:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny, Remember: protocol.RememberSession}, true
	case approvalDenyProject:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny, Remember: protocol.RememberProject}, true
	case approvalDenyGlobal:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny, Remember: protocol.RememberGlobal}, true
	case approvalDenyOnce:
		return conversation.ApprovalAnswer{Decision: protocol.ApprovalDeny}, true
	default:
		return conversation.ApprovalAnswer{}, false
	}
}

func approvalActionFromAnswer(answer conversation.ApprovalAnswer) approvalAction {
	if answer.Decision == protocol.ApprovalDeny {
		switch answer.Remember {
		case protocol.RememberSession:
			return approvalDenySession
		case protocol.RememberProject:
			return approvalDenyProject
		case protocol.RememberGlobal:
			return approvalDenyGlobal
		default:
			return approvalDenyOnce
		}
	}
	switch answer.Remember {
	case protocol.RememberSession:
		return approvalAllowSession
	case protocol.RememberProject:
		return approvalAllowProject
	case protocol.RememberGlobal:
		return approvalAllowGlobal
	default:
		return approvalAllowOnce
	}
}
