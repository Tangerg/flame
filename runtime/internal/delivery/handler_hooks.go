package delivery

import (
	"context"
	"fmt"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// ListHooks reports the lifecycle hooks discovered for a cwd — global
// (~/.flame) + the project's (.flame) — each marked active iff it currently runs
// (global always; project only when the project is trusted). The client renders
// this for review and a trust toggle.
func (s *Handler) ListHooks(ctx context.Context, in protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
	insp, err := s.hooks.Inspect(ctx, in.Workspace.Path)
	if err != nil {
		return nil, wireWorkspaceError(fmt.Errorf("hooks: inspect: %w", err))
	}
	out := &protocol.HooksListResult{
		ProjectRoot:    insp.ProjectRoot,
		ProjectTrusted: insp.ProjectTrusted,
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
		out.Hooks = append(out.Hooks, protocol.HookInfo{
			Event:         event,
			Matcher:       h.Matcher,
			Command:       h.Command,
			Inject:        h.Inject,
			TimeoutMillis: h.TimeoutMillis,
			Scope:         scope,
			Source:        h.Source,
			Active:        resolved.Active,
		})
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
	case hookdata.ScopeProject:
		return protocol.HookScopeProject, true
	default:
		return "", false
	}
}

// SetHookTrust trusts (or revokes) a project's hooks (hooks.
// setTrust). The change takes effect on the next Run — the resolver re-reads
// trust per Run.
func (s *Handler) SetHookTrust(ctx context.Context, in protocol.SetHookTrustRequest) error {
	return wireWorkspaceError(s.hooks.SetProjectTrust(ctx, in.ProjectRoot, in.Trusted))
}
