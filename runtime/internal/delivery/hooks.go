package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	hooksList     Name = "hooks.list"
	hooksSetTrust Name = "hooks.setTrust"
)

func registerHooks(registry *Registry) {
	registry.query(MethodMeta{
		Name: hooksList, GoName: "ListHooks",
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		ListHooks(context.Context, protocol.ListHooksRequest) (*protocol.HooksListResult, error)
	}, ctx context.Context, request protocol.ListHooksRequest) (*protocol.HooksListResult, error) {
		return service.ListHooks(ctx, request)
	})

	registry.commandAck(MethodMeta{
		Name: hooksSetTrust, GoName: "SetHookTrust",
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		SetHookTrust(context.Context, protocol.SetHookTrustRequest) error
	}, ctx context.Context, request protocol.SetHookTrustRequest) error {
		return service.SetHookTrust(ctx, request)
	})
}
