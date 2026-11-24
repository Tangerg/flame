package delivery

import (
	"context"
	"fmt"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (s *Handler) ListHooks(ctx context.Context, in protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
	insp, err := s.hooks.Inspect(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(fmt.Errorf("hooks: inspect: %w", err))
	}
	out := &protocol.HooksListResult{
		RepositoryRoot:    insp.RepositoryRoot,
		RepositoryTrusted: insp.RepositoryTrusted,
		Hooks:          make([]protocol.HookInfo, 0, len(insp.Hooks)),
	}
	for _, resolved := range insp.Hooks {
		h := resolved.Hook
		event, ok := presentHookEvent(h.Event)
		if !ok {
			return nil, fmt.Errorf("hooks.list: unsupported hook event %q", h.Event)
		}
		scope, ok := presentHookScope(h.Scope)
		if !ok {
			return nil, fmt.Errorf("hooks.list: unsupported hook scope %q", h.Scope)
		}
		info := protocol.HookInfo{
			Event:         event,
			Matcher:       h.Matcher,
			Command:       h.Command,
			Inject:        h.Inject,
			TimeoutMillis: h.TimeoutMillis,
			Scope:         scope,
			Source:        h.Source,
			Active:        resolved.Active,
		}
		if h.Command != "" {
			info.FailurePolicy = h.FailurePolicy.Effective()
		}
		out.Hooks = append(out.Hooks, info)
	}
	return out, nil
}

func presentHookEvent(event hookdata.Event) (protocol.HookEvent, bool) {
	switch event {
	case hookdata.PreToolUse:
		return protocol.HookEventPreToolUse, true
	case hookdata.PostToolUse:
		return protocol.HookEventPostToolUse, true
	case hookdata.UserPromptSubmit:
		return protocol.HookEventUserPromptSubmit, true
	case hookdata.SessionStart:
		return protocol.HookEventSessionStart, true
	case hookdata.PreCompact:
		return protocol.HookEventPreCompact, true
	case hookdata.Stop:
		return protocol.HookEventStop, true
	case hookdata.Notification:
		return protocol.HookEventNotification, true
	default:
		return "", false
	}
}

func presentHookScope(scope hookdata.Scope) (protocol.HookScope, bool) {
	switch scope {
	case hookdata.ScopeGlobal:
		return protocol.HookScopeGlobal, true
	case hookdata.ScopeRepository:
		return protocol.HookScopeRepository, true
	default:
		return "", false
	}
}

func (s *Handler) SetHookTrust(ctx context.Context, in protocol.SetHookTrustRequest) error {
	return wireWorkspaceError(s.hooks.SetRepositoryTrust(ctx, in.RepositoryRoot, in.Trusted))
}
