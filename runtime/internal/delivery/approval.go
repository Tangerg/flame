package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	ApprovalGetMode    Name = "approval.getMode"
	ApprovalSetMode    Name = "approval.setMode"
	ApprovalListRules  Name = "approval.listRules"
	ApprovalForgetRule Name = "approval.forgetRule"
)

func registerApproval(registry *Registry) {
	registry.Query(MethodMeta{Name: ApprovalGetMode},
		func(service *Handler, ctx context.Context, _ struct{}) (*protocol.ApprovalModeResult, error) {
			return service.GetApprovalMode(ctx)
		})

	registry.Command(MethodMeta{Name: ApprovalSetMode},
		(*Handler).SetApprovalMode)

	registry.Query(MethodMeta{Name: ApprovalListRules},
		(*Handler).ListApprovalRules)

	registry.CommandAck(MethodMeta{Name: ApprovalForgetRule},
		(*Handler).ForgetApprovalRule)
}
