package delivery

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type pluginCallTarget struct {
	ids         []string
	invocations []capability.Invocation
}

type pluginStreamOwner struct {
	pluginCallUseCases
	err error
	ids []string
}

func (s *pluginStreamOwner) PrepareStream(_ context.Context, _ plugins.CallRequest) (plugins.StreamCall, error) {
	return s, nil
}

func (*pluginStreamOwner) Kind() capability.MethodKind { return capability.Mutation }

func (s *pluginStreamOwner) Stream(ctx context.Context) iter.Seq2[plugins.StreamEvent, error] {
	return func(yield func(plugins.StreamEvent, error) bool) {
		s.ids = append(s.ids, operation.Identity(ctx))
		value, _ := jsoninput.Parse([]byte(`"partial"`))
		if !yield(plugins.StreamEvent{Kind: plugins.StreamChunk, Value: value}, nil) {
			return
		}
		yield(plugins.StreamEvent{}, s.err)
	}
}

func TestPluginStreamProjectsKnownRefusalWithoutSettlingUnknownError(t *testing.T) {
	contract := testsupport.CommandContract(t, "records.stream", "installation")
	request := protocol.CallPluginRequest{InstallationID: testsupport.InstallationID(t).String(), Digest: testsupport.Digest("1").String(), Contribution: "records", Contract: string(contract.JSON()), Method: "records.stream", Input: `{}`, Target: capability.Target{Scope: "installation", Reference: testsupport.InstallationID(t).String()}}
	owner := &pluginStreamOwner{err: &capability.Refusal{Code: "refused", Message: "owner refused"}}
	handler := &Handler{pluginCalls: owner, features: featureAvailability{plugins: true}}
	endpoint := mustNewEndpoint(t, handler, EndpointConfig{})
	for range 2 {
		result := endpoint.Invoke(t.Context(), PluginsStream, request, Options{IdempotencyKey: "original"})
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
		var events []protocol.PluginStreamEvent
		for value, err := range result.Events {
			if err != nil {
				t.Fatal(err)
			}
			events = append(events, value.(protocol.PluginStreamEvent))
		}
		if len(events) != 2 || events[0].Kind != protocol.PluginStreamChunk || events[1].Kind != protocol.PluginStreamFailure || events[1].Failure.Code != "refused" {
			t.Fatalf("known stream=%+v", events)
		}
	}
	if owner.ids[0] == "" || owner.ids[0] != owner.ids[1] {
		t.Fatalf("owner identity changed: %v", owner.ids)
	}
	owner.err = errors.New("outcome unknown")
	result := endpoint.Invoke(t.Context(), PluginsStream, request, Options{IdempotencyKey: "original"})
	var terminal bool
	var failure error
	for value, err := range result.Events {
		if err != nil {
			failure = err
			break
		}
		terminal = value.(protocol.PluginStreamEvent).Kind != protocol.PluginStreamChunk
	}
	if failure == nil || terminal {
		t.Fatalf("unknown error became terminal evidence: %v %v", terminal, failure)
	}
}

func (*pluginCallTarget) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{Capabilities: protocol.ServerCapabilities{Features: map[string]protocol.FeatureCapability{protocol.FeaturePlugins: {Enabled: true}}}}, nil
}

func (s *pluginCallTarget) PreparePluginCall(_ context.Context, request protocol.CallPluginRequest) (OperationKind, func(context.Context) (*protocol.CallPluginResponse, error), error) {
	kind, err := pluginCallOperation(request)
	return kind, func(ctx context.Context) (*protocol.CallPluginResponse, error) {
		s.ids = append(s.ids, operation.Identity(ctx))
		invocation, err := operation.Capture(ctx, request.Target)
		if err != nil {
			return nil, err
		}
		s.invocations = append(s.invocations, invocation)
		return &protocol.CallPluginResponse{Output: `{"revision":2}`}, nil
	}, err
}

func TestPluginCallResolvesQueryAndOwnerCommandPoliciesAtTheSharedEndpoint(t *testing.T) {
	contract, err := capability.Parse(testsupport.CommandContract(t, "review.update", "installation").JSON())
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CallPluginRequest{InstallationID: testsupport.InstallationID(t).String(), Digest: testsupport.Digest("1").String(), Contribution: "review-operations", Contract: string(contract.Identity().JSON()), Target: capability.Target{Scope: "installation", Reference: "12345678-1234-1234-1234-123456789abc"}, Method: "review.update", Input: `{"revision":2}`}
	target := &pluginCallTarget{}
	endpoint := mustNewEndpoint(t, target, EndpointConfig{})
	ctx := operation.Adopt(t.Context(), "parent-tool", capability.Model, "run_owner")
	for range 2 {
		result := endpoint.Invoke(ctx, PluginsCall, request, Options{IdempotencyKey: "original"})
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
	}
	if target.invocations[0].Origin != capability.Model || target.invocations[0].Parent != "parent-tool" || target.invocations[0].Target != request.Target || target.invocations[0] != target.invocations[1] {
		t.Fatalf("nested public call lost its calling owner: %+v", target.invocations)
	}
	if len(target.ids) != 2 || target.ids[0] == "" || target.ids[0] != target.ids[1] {
		t.Fatalf("owner invocation identities = %v", target.ids)
	}
	query := contract.Descriptor()
	query.Methods[0].Kind = capability.Query
	queryContract, err := capability.FromDescriptor(query)
	if err != nil {
		t.Fatal(err)
	}
	request.Contract = string(queryContract.Identity().JSON())
	if result := endpoint.Invoke(t.Context(), PluginsCall, request, Options{IdempotencyKey: "query"}); result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInvalidParams) {
		t.Fatalf("query identity was admitted: %+v", result)
	}
	if len(target.ids) != 2 {
		t.Fatal("rejected query reached its owner")
	}
	if result := endpoint.Invoke(t.Context(), PluginsCall, request, Options{}); result.Failure != nil {
		t.Fatal(result.Failure)
	}
}
