package delivery

import (
	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	HooksList     Name = "hooks.list"
	HooksSetTrust Name = "hooks.setTrust"
)

func registerHooks(registry *Registry) {
	registry.query(MethodMeta{
		Name:   HooksList,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, (*Handler).ListHooks)

	registry.commandAck(MethodMeta{
		Name: HooksSetTrust,
	}, (*Handler).SetHookTrust)
}
