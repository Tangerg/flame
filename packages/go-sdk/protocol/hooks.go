package protocol

import "github.com/Tangerg/flame/packages/go-sdk/hook"

type ListHooksRequest struct {
	Workspace WorkspaceRef `json:"workspace"`
}

//flame:enum
type HookEvent string

const (
	HookEventPreToolUse       = HookEvent(hook.PreToolUse)
	HookEventPostToolUse      = HookEvent(hook.PostToolUse)
	HookEventUserPromptSubmit = HookEvent(hook.UserPromptSubmit)
	HookEventSessionStart     = HookEvent(hook.SessionStart)
	HookEventPreCompact       = HookEvent(hook.PreCompact)
	HookEventStop             = HookEvent(hook.Stop)
	HookEventNotification     = HookEvent(hook.Notification)
)

type HookInfo struct {
	Event         HookEvent          `json:"event"`
	Matcher       string             `json:"matcher,omitempty"`
	Command       string             `json:"command,omitempty"`
	Inject        string             `json:"inject,omitempty"`
	TimeoutMillis int                `json:"timeoutMillis,omitzero"`
	FailurePolicy hook.FailurePolicy `json:"failurePolicy,omitempty"`
	Scope         HookScope          `json:"scope"`
	Source        string             `json:"source"`
	Active        bool               `json:"active"`
}

//flame:enum
type HookScope string

const (
	HookScopeGlobal     HookScope = "global"
	HookScopeRepository HookScope = "repository"
)

type HooksListResult struct {
	RepositoryRoot    string     `json:"repositoryRoot,omitempty"`
	RepositoryTrusted bool       `json:"repositoryTrusted"`
	Hooks             []HookInfo `json:"hooks"`
}

type SetHookTrustRequest struct {
	RepositoryRoot string `json:"repositoryRoot"`
	Trusted        bool   `json:"trusted"`
}
