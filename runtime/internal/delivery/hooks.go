package delivery

import (
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	HooksList     Name = "hooks.list"
	HooksSetTrust Name = "hooks.setTrust"
)

func registerHooks(registry *Registry) {
	registry.Query(MethodMeta{
		Name:   HooksList,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, (*Handler).ListHooks)

	registry.CommandAck(MethodMeta{
		Name: HooksSetTrust,
	}, (*Handler).SetHookTrust)
}
