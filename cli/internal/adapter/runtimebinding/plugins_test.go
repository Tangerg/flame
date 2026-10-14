package runtimebinding

import (
	"context"
	"errors"
	"slices"
	"testing"

	flameruntime "github.com/Tangerg/flame/runtime"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

type pluginBindingStub struct {
	pluginBinding
	page        *protocol.Page[protocol.PluginInstallation]
	selected    *protocol.PluginInstallation
	provider    *protocol.PluginProviderSelection
	composition *protocol.PluginComposition
}

func (s pluginBindingStub) SetPluginComposition(context.Context, protocol.SetPluginCompositionRequest, flameruntime.CommandOptions) (*protocol.PluginComposition, error) {
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

func (s pluginBindingStub) SelectPluginProvider(context.Context, protocol.SelectPluginProviderRequest, flameruntime.CommandOptions) (*protocol.PluginProviderSelection, error) {
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

func (s pluginBindingStub) ListPlugins(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.PluginInstallation], error) {
	return s.page, nil
}

func (s pluginBindingStub) SelectPlugin(context.Context, protocol.PluginReleaseRequest, flameruntime.CommandOptions) (*protocol.PluginInstallation, error) {
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
