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
	ids []string
}

func (*pluginCallTarget) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{Capabilities: protocol.ServerCapabilities{Features: map[string]protocol.FeatureCapability{protocol.FeaturePlugins: {Enabled: true}}}}, nil
}

func (s *pluginCallTarget) PreparePluginCall(_ context.Context, request protocol.CallPluginRequest) (OperationKind, func(context.Context) (*protocol.CallPluginResponse, error), error) {
	kind, err := pluginCallOperation(request)
	return kind, func(ctx context.Context) (*protocol.CallPluginResponse, error) {
		s.ids = append(s.ids, operation.Identity(ctx))
		return &protocol.CallPluginResponse{Output: []byte(`{"revision":2}`)}, nil
	}, err
}

func TestPluginCallResolvesQueryAndOwnerCommandPoliciesAtTheSharedEndpoint(t *testing.T) {
	contract, err := capability.Parse(testsupport.CommandContract(t, "review.update", "installation").JSON())
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CallPluginRequest{InstallationID: testsupport.InstallationID(t).String(), Digest: testsupport.Digest("1").String(), Contribution: "review-operations", Contract: contract.Descriptor(), Method: "review.update", Input: []byte(`{"revision":2}`)}
	target := &pluginCallTarget{}
	endpoint := mustNewEndpoint(t, target, EndpointConfig{})
	for range 2 {
		result := endpoint.Invoke(t.Context(), PluginsCall, request, Options{IdempotencyKey: "original"})
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
	}
	if len(target.ids) != 2 || target.ids[0] == "" || target.ids[0] != target.ids[1] {
		t.Fatalf("owner invocation identities = %v", target.ids)
	}
	query := request.Contract
	query.Methods[0].Kind = capability.Query
	request.Contract = query
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
