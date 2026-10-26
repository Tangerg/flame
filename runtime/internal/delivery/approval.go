package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	approvalGetMode    Name = "approval.getMode"
	approvalSetMode    Name = "approval.setMode"
	ApprovalListRules  Name = "approval.listRules"
	approvalSetRule    Name = "approval.setRule"
	approvalForgetRule Name = "approval.forgetRule"
)

func registerApproval(registry *Registry) {
	registry.commandAck(MethodMeta{Name: approvalSetRule, GoName: "SetApprovalRule"}, func(service interface {
		SetApprovalRule(context.Context, protocol.SetApprovalRuleRequest) error
	}, ctx context.Context, request protocol.SetApprovalRuleRequest) error {
		return service.SetApprovalRule(ctx, request)
	})

	registry.query(MethodMeta{Name: approvalGetMode, GoName: "GetApprovalMode"},
		func(service interface {
			GetApprovalMode(context.Context) (*protocol.ApprovalModeResult, error)
		}, ctx context.Context, _ struct{}) (*protocol.ApprovalModeResult, error) {
			return service.GetApprovalMode(ctx)
		})

	registry.command(MethodMeta{Name: approvalSetMode, GoName: "SetApprovalMode"},
		func(service interface {
			SetApprovalMode(context.Context, protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error)
		}, ctx context.Context, request protocol.SetApprovalModeRequest) (*protocol.ApprovalModeResult, error) {
			return service.SetApprovalMode(ctx, request)
		})

	registry.query(MethodMeta{Name: ApprovalListRules, GoName: "ListApprovalRules"},
		func(service interface {
			ListApprovalRules(context.Context, protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error)
		}, ctx context.Context, request protocol.ListApprovalRulesRequest) (*protocol.ListApprovalRulesResult, error) {
			return service.ListApprovalRules(ctx, request)
		})

	registry.commandAck(MethodMeta{Name: approvalForgetRule, GoName: "ForgetApprovalRule"},
		func(service interface {
			ForgetApprovalRule(context.Context, protocol.ForgetApprovalRuleRequest) error
		}, ctx context.Context, request protocol.ForgetApprovalRuleRequest) error {
			return service.ForgetApprovalRule(ctx, request)
		})
}
