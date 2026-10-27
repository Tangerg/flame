package runtimebinding

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/portable"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

type pluginBindingStub struct {
	pluginBinding
	page        *protocol.Page[protocol.PluginInstallation]
	selected    *protocol.PluginInstallation
	provider    *protocol.PluginProviderSelection
	composition *protocol.PluginComposition
	lookup      func(context.Context, protocol.PluginCommandRequest, protocol.CallOptions) (*protocol.CallPluginRequest, error)
}

func (s pluginBindingStub) SetPluginComposition(context.Context, protocol.SetPluginCompositionRequest, protocol.CommandOptions) (*protocol.PluginComposition, error) {
	return s.composition, nil
}

func TestPluginCompositionRefusesAnotherContractOrMemberOrder(t *testing.T) {
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.SetPluginCompositionRequest{Contract: string(contract.Identity().JSON()), Members: []protocol.PluginCompositionMember{{InstallationID: "first", Contribution: "outer"}, {InstallationID: "second", Contribution: "inner"}}}
	reversed := slices.Clone(request.Members)
	slices.Reverse(reversed)
	for name, value := range map[string]*protocol.PluginComposition{
		"absent":           nil,
		"invalid contract": {Contract: `{}`, Members: request.Members},
		"reversed":         {Contract: request.Contract, Members: reversed},
		"original":         {Contract: request.Contract, Members: request.Members},
	} {
		connection := &Connection{plugins: pluginBindingStub{composition: value}, meta: requestMeta("test")}
		actual, err := connection.SetPluginComposition(t.Context(), request, "")
		if name == "original" {
			if err != nil || actual != value {
				t.Fatalf("original composition refused: %+v, %v", actual, err)
			}
		} else if !errors.Is(err, conversation.ErrIncompatibleRuntime) {
			t.Fatalf("%s composition accepted: %+v, %v", name, actual, err)
		}
	}
}

func (s pluginBindingStub) SelectPluginProvider(context.Context, protocol.SelectPluginProviderRequest, protocol.CommandOptions) (*protocol.PluginProviderSelection, error) {
	return s.provider, nil
}

func TestPluginProviderSelectionRefusesAnotherBinding(t *testing.T) {
	request := protocol.SelectPluginProviderRequest{Role: "memory", InstallationID: "requested", Contribution: "memory"}
	for _, value := range []*protocol.PluginProviderSelection{nil, {Role: "other", InstallationID: "requested", Contribution: "memory"}, {Role: "memory", InstallationID: "other", Contribution: "memory"}, {Role: "memory", InstallationID: "requested", Contribution: "other"}} {
		connection := &Connection{plugins: pluginBindingStub{provider: value}, meta: requestMeta("test")}
		if _, err := connection.SelectPluginProvider(t.Context(), request, ""); !errors.Is(err, conversation.ErrIncompatibleRuntime) {
			t.Fatalf("another provider accepted: %+v, %v", value, err)
		}
	}
}

func (s pluginBindingStub) ListPlugins(context.Context, protocol.CallOptions) (*protocol.Page[protocol.PluginInstallation], error) {
	return s.page, nil
}

func (s pluginBindingStub) SelectPlugin(context.Context, protocol.PluginReleaseRequest, protocol.CommandOptions) (*protocol.PluginInstallation, error) {
	return s.selected, nil
}

func TestPluginCatalogRefusesAnIncompleteOrRepeatedPage(t *testing.T) {
	t.Parallel()
	for name, page := range map[string]*protocol.Page[protocol.PluginInstallation]{
		"nil":       nil,
		"truncated": protocol.NewPageWithCursor([]protocol.PluginInstallation{{ID: "a"}}, "next"),
		"repeated":  protocol.NewPage([]protocol.PluginInstallation{{ID: "a"}, {ID: "a"}}),
	} {
		runtime := &Connection{plugins: pluginBindingStub{page: page}, meta: requestMeta("test")}
		if _, err := runtime.ListPlugins(t.Context()); !errors.Is(err, conversation.ErrIncompatibleRuntime) {
			t.Fatalf("%s plugin page = %v", name, err)
		}
	}
}

func TestPluginMutationRefusesAnotherInstallationsResult(t *testing.T) {
	t.Parallel()
	request := protocol.PluginReleaseRequest{InstallationID: "requested", Digest: "digest"}
	for name, result := range map[string]*protocol.PluginInstallation{
		"nil":   nil,
		"other": {ID: "other"},
	} {
		runtime := &Connection{plugins: pluginBindingStub{selected: result}, meta: requestMeta("test")}
		if _, err := runtime.SelectPlugin(t.Context(), request, ""); !errors.Is(err, conversation.ErrIncompatibleRuntime) {
			t.Fatalf("%s select result = %v", name, err)
		}
	}
}

func (s pluginBindingStub) GetPluginCommand(ctx context.Context, query protocol.PluginCommandRequest, options protocol.CallOptions) (*protocol.CallPluginRequest, error) {
	return s.lookup(ctx, query, options)
}

func TestPluginCommandLookupPreservesScopeInputAndFailure(t *testing.T) {
	const installation = "12345678-1234-1234-1234-123456789abc"
	original := protocol.CallPluginRequest{InstallationID: installation, Digest: strings.Repeat("a", 64), Contribution: "retained", Contract: `{}`, Method: "test.retained/save.v1", Input: `{"scale":1.0,"value":18446744073709551615}`, Target: portable.Target{Scope: "installation", Reference: installation}}
	invalid := original
	invalid.Digest = "changed"
	upstream := errors.New("original route unavailable")
	query := protocol.PluginCommandRequest{Namespace: "idp_" + strings.Repeat("1", 32), Key: "original-command"}
	for _, test := range []struct {
		name    string
		value   *protocol.CallPluginRequest
		failure error
	}{
		{name: "original", value: &original},
		{name: "nil"},
		{name: "invalid", value: &invalid},
		{name: "upstream error", failure: upstream},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			var connection *Connection
			calls := 0
			stub := pluginBindingStub{lookup: func(gotContext context.Context, got protocol.PluginCommandRequest, options protocol.CallOptions) (*protocol.CallPluginRequest, error) {
				calls++
				if gotContext != ctx || got != query || !reflect.DeepEqual(options, connection.callOptions()) {
					t.Fatalf("lookup changed its caller scope: %+v %+v", got, options)
				}
				return test.value, test.failure
			}}
			connection = &Connection{plugins: stub, meta: requestMeta("original command inspection")}
			actual, err := connection.GetPluginCommand(ctx, query)
			if calls != 1 {
				t.Fatalf("lookup calls = %d", calls)
			}
			switch test.name {
			case "original":
				if err != nil || actual != &original || actual.Input != original.Input {
					t.Fatalf("original request changed: %+v, %v", actual, err)
				}
			case "upstream error":
				if !errors.Is(err, upstream) {
					t.Fatalf("lookup lost its cause: %v", err)
				}
			default:
				if !errors.Is(err, conversation.ErrIncompatibleRuntime) {
					t.Fatalf("invalid lookup response accepted: %+v, %v", actual, err)
				}
			}
		})
	}
}
