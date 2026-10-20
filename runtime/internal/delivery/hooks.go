package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	HooksList     Name = "hooks.list"
	HooksSetTrust Name = "hooks.setTrust"
)

func registerHooks(registry *Registry) {
	registry.query(MethodMeta{
		Name: HooksList, GoName: "ListHooks",
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		ListHooks(context.Context, protocol.ListHooksRequest) (*protocol.HooksListResult, error)
	}, ctx context.Context, request protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
		return service.ListHooks(ctx, request)
	})

	registry.commandAck(MethodMeta{
		Name: HooksSetTrust, GoName: "SetHookTrust",
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		SetHookTrust(context.Context, protocol.SetHookTrustRequest) error
	}, ctx context.Context, request protocol.SetHookTrustRequest) error {
		return service.SetHookTrust(ctx, request)
	})
}
