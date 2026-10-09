package runtimebinding

import (
	"context"
	"errors"
	"testing"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

type pluginBindingStub struct {
	pluginBinding
	page     *protocol.Page[protocol.PluginInstallation]
	selected *protocol.PluginInstallation
	renamed  *protocol.Session
}

func (s pluginBindingStub) RenamePluginSession(context.Context, protocol.RenamePluginSessionRequest, flameruntime.CommandOptions) (*protocol.Session, error) {
	return s.renamed, nil
}

func TestPluginRenameRefusesAnotherSessionsResult(t *testing.T) {
	request := protocol.RenamePluginSessionRequest{Update: protocol.SessionTitleEdit{SessionID: "requested"}}
	for _, result := range []*protocol.Session{nil, {ID: "other"}} {
		runtime := &Connection{plugins: pluginBindingStub{renamed: result}, meta: requestMeta("test")}
		if _, err := runtime.RenamePluginSession(t.Context(), request, ""); !errors.Is(err, conversation.ErrIncompatibleRuntime) {
			t.Fatalf("rename result: %v", err)
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
