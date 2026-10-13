package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

type pluginCallTarget struct {
	ids         []string
	invocations []capability.Invocation
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
		return &protocol.CallPluginResponse{Output: []byte(`{"revision":2}`)}, nil
	}, err
}

func TestPluginCallResolvesQueryAndOwnerCommandPoliciesAtTheSharedEndpoint(t *testing.T) {
	contract, err := capability.Parse(testsupport.CommandContract(t, "review.update", "installation").JSON())
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CallPluginRequest{InstallationID: testsupport.InstallationID(t).String(), Digest: testsupport.Digest("1").String(), Contribution: "review-operations", Contract: string(contract.Identity().JSON()), Target: capability.Target{Scope: "installation", Reference: "12345678-1234-1234-1234-123456789abc"}, Method: "review.update", Input: []byte(`{"revision":2}`)}
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
