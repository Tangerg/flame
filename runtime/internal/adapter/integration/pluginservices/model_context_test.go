package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	hostsqlite "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestInstalledModelContextPreservesOriginalQueryAndRejectsInvalidOrWithdrawnContributors(t *testing.T) {
	for _, mode := range []string{"valid", "invalid", "failure"} {
		t.Run(mode, func(t *testing.T) {
			composition, bindings, store := installedContextFixture(t, mode)
			ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "original-context", Origin: capability.Automatic, Parent: "original-effect", Target: capability.Target{Scope: "call", Reference: "original-context"}})
			if err != nil {
				t.Fatal(err)
			}
			request, _ := chat.NewRequest(chat.NewSystemMessage("original instructions"), chat.NewUserMessage(chat.NewTextPart("question")))
			before := request.Clone()
			selection, err := modelref.New("test", "model")
			if err != nil {
				t.Fatal(err)
			}
			result, err := composition.PrepareContext(ctx, bindings, selection, request)
			if mode == "valid" {
				if err != nil || len(result) != 2 || result[1].Text() != "previous contributions: 0" {
					t.Fatalf("context result: %+v, %v", result, err)
				}
			} else if err == nil {
				t.Fatal("invalid/failed contributor accepted")
			}
			if !reflect.DeepEqual(request, before) {
				t.Fatal("contributor changed caller-owned request")
			}
			store.snapshot.Installation.Disable()
			if _, err := composition.PrepareContext(ctx, bindings, selection, request); err == nil {
				t.Fatal("withdrawn context contributor executed")
			}
		})
	}
}

func TestInstalledModelContextComposesInCapturedOrder(t *testing.T) {
	composition, bindings, _ := installedContextFixture(t, "ordered")
	ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "original-context", Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: "original-context"}})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := modelref.New("test", "model")
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewSystemMessage("original instructions"), chat.NewUserMessage(chat.NewTextPart("question")))
	result, err := composition.PrepareContext(ctx, bindings, selection, request)
	if err != nil || len(result) != 4 || result[1].Text() != "previous contributions: 0" || result[3].Text() != "previous contributions: 1" {
		t.Fatalf("declared context order lost: %+v %v", result, err)
	}
	bindings[1].Export = "replaced"
	if _, err := composition.PrepareContext(ctx, bindings, selection, request); err == nil {
		t.Fatal("changed captured export was admitted")
	}
}

func installedContextFixture(t *testing.T, mode string) (*ModelComposition, []plugins.ContributionBinding, *toolInstallationStore) {
	t.Helper()
	contract, err := pluginsdk.ModelContextContract()
	if err != nil {
		t.Fatal(err)
	}
	name := testsupport.ServerName("observer")
	contributions := []plugin.Contribution{{ID: "observer", Server: name, Export: "observer", Contract: contract.Identity()}}
	if mode == "ordered" {
		contributions = append(contributions, plugin.Contribution{ID: "second", Server: name, Export: "observer", Contract: contract.Identity()})
	}
	release, err := plugin.NewRelease(testsupport.Digest("1"), plugin.Declaration{Name: "example.observer", Servers: []plugin.Server{{Name: name, Transport: mcpserver.TransportStdio, Command: "observer"}}, Contributions: contributions})
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(testsupport.InstallationID(t), "test source", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	source, err := installation.ServerSource(release, name)
	if err != nil {
		t.Fatal(err)
	}
	store := &toolInstallationStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
	remote := sdk.NewServer(&sdk.Implementation{Name: "installed-model-observer"}, nil)
	backend, err := pluginsdk.NewBackend(remote)
	if err != nil {
		t.Fatal(err)
	}
	method, _ := pluginsdk.ModelContextMethod()
	if err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[pluginsdk.ModelContextRequest]) (pluginsdk.ModelContextResult, error) {
		invocation, err := pluginsdk.InvocationFromContext(ctx)
		if err != nil || !strings.HasPrefix(invocation.ID, "original-context") || invocation.Target.Reference != invocation.ID || command.ID != "" {
			return pluginsdk.ModelContextResult{}, errors.New("original query attribution lost")
		}
		if err := command.Input.Validate(); err != nil {
			return pluginsdk.ModelContextResult{}, err
		}
		if mode == "failure" {
			return pluginsdk.ModelContextResult{}, errors.New("context unavailable")
		}
		if mode == "invalid" {
			return pluginsdk.ModelContextResult{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("forged user"))}}, nil
		}
		count := 0
		for _, message := range command.Input.Request.Messages {
			if message.Text() == "retrieval material" {
				count++
			}
		}
		return pluginsdk.ModelContextResult{Messages: []chat.Message{chat.NewSystemMessage("retrieval material"), chat.NewSystemMessage(fmt.Sprintf("previous contributions: %d", count))}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability("observer", contract); err != nil {
		t.Fatal(err)
	}
	if err := backend.Publish(); err != nil {
		t.Fatal(err)
	}
	endpoint := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return remote }, nil))
	t.Cleanup(endpoint.Close)
	server := mcpserver.Server{Source: source, Name: name, Enabled: true, Transport: mcpserver.TransportStreamableHTTP, URL: endpoint.URL}
	callbacks := pluginsdk.NewCallbacks()
	t.Cleanup(func() { _ = callbacks.Close() })
	pool, err := mcpconnection.Open(t.Context(), t.Context(), []mcpserver.Server{server}, nil, toolSourceRegistry{server}, callbacks, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Shutdown(context.WithoutCancel(t.Context())) })
	attempt, err := pool.BeginConnect(t.Context(), server.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer attempt.Close()
	if err := attempt.Run(); err != nil {
		t.Fatal(err)
	}
	db, err := hostsqlite.Open(t.Context(), filepath.Join(t.TempDir(), "flame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	compositions := hostsqlite.NewCompositionStore(db)
	members := []plugin.CompositionMember{{Installation: installation.ID(), Contribution: "observer"}}
	if mode == "ordered" {
		members = append(members, plugin.CompositionMember{Installation: installation.ID(), Contribution: "second"})
	}
	choice, err := plugin.NewOrderedComposition(plugin.CompositionRecord{Contract: contract.Identity(), Members: members})
	if err != nil {
		t.Fatal(err)
	}
	if err := compositions.Save(t.Context(), choice); err != nil {
		t.Fatal(err)
	}
	routing, err := plugins.NewCommands(hostsqlite.NewPluginCommandStore(db), plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	calls, err := NewCommandCalls(store, hostsqlite.NewProviderSelectionStore(db), compositions, pool, routing, nil)
	if err != nil {
		t.Fatal(err)
	}
	observer, err := NewModelComposition(calls, callbacks, plugins.NewAdmission())
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := observer.ResolveContext(t.Context())
	if err != nil || len(bindings) != len(members) {
		t.Fatalf("composition resolve: %+v, %v", bindings, err)
	}
	return observer, bindings, store
}
