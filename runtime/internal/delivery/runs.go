package delivery

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	RunsStart                 Name = "runs.start"
	RunsResume                Name = "runs.resume"
	runsSubscribe             Name = "runs.subscribe"
	RunsCancel                Name = "runs.cancel"
	RunsSteer                 Name = "runs.steer"
	RunsGet                   Name = "runs.get"
	RunsGetAcceptance         Name = "runs.getAcceptance"
	RunsList                  Name = "runs.list"
	modelInvocationsList      Name = "modelInvocations.list"
	modelInvocationsEndpoints Name = "modelInvocations.endpoints"
)

func registerRuns(registry *Registry) {
	registry.query(MethodMeta{Name: modelInvocationsEndpoints, GoName: "ListModelEndpoints", Errors: []string{protocol.ErrCapabilityNotNeg.Error()}}, func(service interface {
		ListModelEndpoints(context.Context, protocol.ListModelEndpointsRequest) (*protocol.Page[protocol.ModelEndpointInvocation], error)
	}, ctx context.Context, request protocol.ListModelEndpointsRequest) (*protocol.Page[protocol.ModelEndpointInvocation], error) {
		return service.ListModelEndpoints(ctx, request)
	})
	registry.query(MethodMeta{Name: modelInvocationsList, GoName: "ListModelInvocations", Errors: []string{protocol.ErrRunNotFound.Error(), protocol.ErrCapabilityNotNeg.Error()}}, func(service interface {
		ListModelInvocations(context.Context, protocol.ListModelInvocationsRequest) (*protocol.Page[protocol.ModelInvocation], error)
	}, ctx context.Context, request protocol.ListModelInvocationsRequest) (*protocol.Page[protocol.ModelInvocation], error) {
		return service.ListModelInvocations(ctx, request)
	})

	registry.runStreamOwnerCommand(MethodMeta{
		Name: RunsStart, GoName: "StartRun",
		Errors: pluginProviderErrors(protocol.ErrSessionNotFound.
			Error(), protocol.ErrProjectNotFound.
			Error(), protocol.ErrSessionBusy.
			Error(), protocol.ErrSessionHasActiveRun.
			Error(), protocol.ErrPromptSourceTooLarge.
			Error(), protocol.ErrUnsupportedMime.
			Error(), protocol.ErrCapabilityNotNeg.
			Error(), protocol.ErrPluginChanged.
			Error(),
		),
	}, func(service interface {
		StartRun(context.Context, protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	}, ctx context.Context, request protocol.StartRunRequest,
	) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		return service.StartRun(ctx, request)
	})

	registry.runStreamOwnerCommand(MethodMeta{
		Name: RunsResume, GoName: "ResumeRun",
		Acknowledgement: []AcknowledgementMatch{
			{Request: runIDField, Result: runIDField, Compare: AcknowledgementEcho},
			{Request: "input", Result: "userItemId", Compare: AcknowledgementPresence},
		},
		Errors: []string{protocol.ErrRunNotFound.
			Error(), protocol.ErrInterruptNotOpen.
			Error(), protocol.ErrCapabilityNotNeg.
			Error(), protocol.ErrPluginChanged.
			Error(),
		},
	}, func(service interface {
		ResumeRun(context.Context, protocol.ResumeRunRequest) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	}, ctx context.Context, request protocol.ResumeRunRequest,
	) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		return service.ResumeRun(ctx, request)
	})

	registry.runSubscription(MethodMeta{
		Name: runsSubscribe, GoName: "SubscribeRun",
		Acknowledgement: []AcknowledgementMatch{
			{Request: runIDField, Result: runIDField, Compare: AcknowledgementEcho},
			{Request: segmentIDField, Result: segmentIDField, Compare: AcknowledgementEcho},
			{Request: "snapshot", Result: "snapshot", Compare: AcknowledgementPresence},
		},
		Errors: []string{protocol.ErrRunNotFound.
			Error(), protocol.ErrRunNotRoot.
			Error(), protocol.ErrRunWaiting.
			Error(), protocol.ErrRunFinished.
			Error(), protocol.ErrStaleSegment.
			Error(), protocol.ErrReplayCursorInvalid.
			Error(), protocol.ErrReplayUnavailable.
			Error(), protocol.ErrCapabilityNotNeg.
			Error(),
		},
	}, func(service interface {
		SubscribeRun(context.Context, protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error)
	}, ctx context.Context, request protocol.SubscribeRunRequest,
	) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
		return service.SubscribeRun(ctx, request)
	})

	registry.command(MethodMeta{
		Name: RunsCancel, GoName: "CancelRun",
		Errors: []string{protocol.ErrRunNotFound.
			Error(), protocol.ErrRunFinished.
			Error(), protocol.ErrSessionBusy.
			Error(), protocol.ErrCapabilityNotNeg.
			Error(),
		},
	}, func(service interface {
		CancelRun(context.Context, protocol.CancelRunRequest) (*protocol.CancelRunResponse, error)
	}, ctx context.Context, request protocol.CancelRunRequest,
	) (*protocol.CancelRunResponse, error) {
		return service.CancelRun(ctx, request)
	}).withOwnedReplay((interface {
		replayCancellation(context.Context, protocol.CancelRunRequest, keyedCall) protocol.OperationResult
	}).replayCancellation)

	registry.command(MethodMeta{
		Name: RunsSteer, GoName: "SteerRun",
		Idempotency: IdempotencyOwnerRun,
		Errors: []string{protocol.ErrRunNotFound.
			Error(), protocol.ErrRunNotRoot.
			Error(), protocol.ErrRunWaiting.
			Error(), protocol.ErrRunFinished.
			Error(), protocol.ErrStaleSegment.
			Error(),
		},
	}, func(service interface {
		SteerRun(context.Context, protocol.SteerRunRequest) (*protocol.SteerRunResponse, error)
	}, ctx context.Context, request protocol.SteerRunRequest,
	) (*protocol.SteerRunResponse, error) {
		return service.SteerRun(ctx, request)
	})

	registry.query(MethodMeta{
		Name: RunsGet, GoName: "GetRun",
		Errors: []string{protocol.ErrRunNotFound.
			Error(), protocol.ErrCapabilityNotNeg.
			Error(),
		},
	}, func(service interface {
		GetRun(context.Context, protocol.GetRunRequest) (*protocol.RunRef, error)
	}, ctx context.Context, request protocol.GetRunRequest,
	) (*protocol.RunRef, error) {
		return service.GetRun(ctx, request)
	})

	registry.query(MethodMeta{Name: RunsGetAcceptance, GoName: "GetRunAcceptance"}, func(service interface {
		GetRunAcceptance(context.Context, protocol.GetRunAcceptanceRequest) (*protocol.RunAcceptance, error)
	}, ctx context.Context, request protocol.GetRunAcceptanceRequest,
	) (*protocol.RunAcceptance, error) {
		return service.GetRunAcceptance(ctx, request)
	})

	registry.query(MethodMeta{
		Name: RunsList, GoName: "ListRuns",
		CapabilityRules: descendantsRequireChildRuns(includeDescendantsParameter),
	}, func(service interface {
		ListRuns(context.Context, protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error)
	}, ctx context.Context, request protocol.ListRunsRequest,
	) (*protocol.Page[protocol.RunRef], error) {
		return service.ListRuns(ctx, request)
	})
}
