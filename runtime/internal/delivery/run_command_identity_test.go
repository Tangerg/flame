package delivery

import (
	"context"
	"errors"
	"iter"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/protocol"
)

func TestResumeFingerprintCoversMeaningAndCanonicalSets(t *testing.T) {
	request := protocol.ResumeRunRequest{RunID: "run_1", Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "input"}}, Responses: []protocol.InterruptResponse{
		{ItemID: "item_approval", Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalDeny, Reason: "denied", Remember: &protocol.RememberScope{Scope: protocol.RememberGlobal}}},
		{ItemID: "item_question", Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"answer"}}}},
	}}
	capabilities := &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval, protocol.InterruptQuestion}}
	fingerprint, err := runCommandFingerprint(RunsResume, request, capabilities)
	if err != nil {
		t.Fatal(err)
	}
	reordered := request
	reordered.Responses = slices.Clone(request.Responses)
	slices.Reverse(reordered.Responses)
	otherOrder := *capabilities
	otherOrder.InterruptTypes = []protocol.InterruptType{protocol.InterruptQuestion, protocol.InterruptApproval}
	otherOrder.ExcludedEphemeralEvents = []protocol.SuppressibleRunEventType{protocol.SuppressibleRunItemDelta}
	if same, err := runCommandFingerprint(RunsResume, reordered, &otherOrder); err != nil || same != fingerprint {
		t.Fatalf("request ordering or observation filtering changed identity: %q,%v", same, err)
	}
	for _, change := range []func(*protocol.ResumeRunRequest){
		func(r *protocol.ResumeRunRequest) { r.RunID = "run_other" },
		func(r *protocol.ResumeRunRequest) { r.Responses[0].ItemID = "item_other" },
		func(r *protocol.ResumeRunRequest) { r.Responses[0].Response.Decision = protocol.ApprovalApprove },
		func(r *protocol.ResumeRunRequest) { r.Responses[0].Response.Reason = "different" },
		func(r *protocol.ResumeRunRequest) {
			r.Responses[0].Response.EditedArgs = map[string]any{"command": "changed"}
		},
		func(r *protocol.ResumeRunRequest) {
			r.Responses[0].Response.Remember = &protocol.RememberScope{Scope: protocol.RememberSession}
		},
		func(r *protocol.ResumeRunRequest) { r.Responses[1].Response.Answers = [][]string{{"different"}} },
		func(r *protocol.ResumeRunRequest) {
			r.Input = []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "different"}}
		},
	} {
		changed := request
		changed.Responses = slices.Clone(request.Responses)
		change(&changed)
		if digest, err := runCommandFingerprint(RunsResume, changed, capabilities); err != nil || digest == fingerprint {
			t.Fatalf("meaning changed without changing identity: %q,%v", digest, err)
		}
	}
	changedCapabilities := *capabilities
	changedCapabilities.Features = map[string]protocol.FeaturePreference{protocol.FeatureSubagents: {Enabled: true}}
	if digest, err := runCommandFingerprint(RunsResume, request, &changedCapabilities); err != nil || digest == fingerprint {
		t.Fatalf("admission context missing from identity: %q,%v", digest, err)
	}
}

type blockedOpeningService struct {
	calls   atomic.Int32
	proceed chan struct{}
}

func (s *blockedOpeningService) StartRun(context.Context, protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	if s.calls.Add(1) == 1 {
		<-s.proceed
	}
	return &protocol.StartRunResponse{RunID: "run_1", SegmentID: "seg_1", UserItemID: "item_1"}, nil, nil
}

func TestOwnerRunCommandSerializesUntilOpeningAcknowledgement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service := &blockedOpeningService{proceed: make(chan struct{})}
		endpoint := mustNewEndpoint(t, service, EndpointConfig{})
		request := protocol.StartRunRequest{SessionID: "ses_1", Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "input"}}}
		results := make(chan Result, 2)
		for range 2 {
			go func() {
				results <- endpoint.Invoke(t.Context(), RunsStart, request, Options{IdempotencyKey: "same-command"})
			}()
		}
		synctest.Wait()
		if service.calls.Load() != 1 {
			t.Fatal("same-key command entered admission before the opening settled")
		}
		close(service.proceed)
		for range 2 {
			if result := <-results; result.Failure != nil {
				t.Fatal(result.Failure)
			}
		}
		if service.calls.Load() != 2 {
			t.Fatal("delivery cached an owner-backed outcome")
		}
	})
}

type runIdentityService struct{ calls int }

func (s *runIdentityService) StartRun(context.Context, protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	s.calls++
	return &protocol.StartRunResponse{RunID: "run_1", SegmentID: "seg_1", UserItemID: "item_1"}, nil, nil
}

func (s *runIdentityService) ResumeRun(context.Context, protocol.ResumeRunRequest) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	s.calls++
	return &protocol.ResumeRunResponse{RunID: "run_1", SegmentID: "seg_2"}, nil, nil
}

func (s *runIdentityService) SteerRun(context.Context, protocol.SteerRunRequest) (*protocol.SteerRunResponse, error) {
	s.calls++
	return &protocol.SteerRunResponse{UserItemID: "item_steer"}, nil
}

func TestOwnerRunCommandsRequireCallerIdentityBeforeDispatch(t *testing.T) {
	for _, test := range []struct {
		method  Name
		request any
	}{
		{RunsStart, protocol.StartRunRequest{SessionID: "ses_1", Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "input"}}}},
		{RunsResume, protocol.ResumeRunRequest{RunID: "run_1", Responses: []protocol.InterruptResponse{{ItemID: "item_question", Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"answer"}}}}}}},
		{RunsSteer, protocol.SteerRunRequest{RunID: "run_1", ExpectedSegmentID: "seg_1", Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "input"}}}},
	} {
		t.Run(string(test.method), func(t *testing.T) {
			service := &runIdentityService{}
			endpoint := mustNewEndpoint(t, service, EndpointConfig{})
			result := endpoint.Invoke(t.Context(), test.method, test.request, Options{})
			if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInvalidParams) || service.calls != 0 {
				t.Fatalf("unkeyed command = %+v, calls = %d; want refusal before dispatch", result, service.calls)
			}
		})
	}
}
