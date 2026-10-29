package execution

import (
	"bytes"
	"context"
	"errors"
	"github.com/Tangerg/flame/qimu"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

var (
	dependedInstallation  = mustInstallationID("940ac827-b431-455b-af4b-e3a170bcfda0")
	unrelatedInstallation = mustInstallationID("1d7f3c2e-6a1b-4f0e-9c3d-2b8a7e5f4c10")
)

func mustInstallationID(text string) identity.InstallationID {
	id, err := identity.ParseInstallation(text)
	if err != nil {
		panic(err)
	}
	return id
}

type pendingInstallationCheckpoints map[identity.InstallationID]bool

func (pendingInstallationCheckpoints) ID() string { return "fixture.checkpoints" }

func (p pendingInstallationCheckpoints) Held(_ context.Context, id qimu.InstallationID) ([]qimu.Dependency, error) {
	for installation, pending := range p {
		if pending && installation.String() == id.String() {
			return []qimu.Dependency{{InstallationID: id, Release: qimu.Digest(testsupport.Digest("checkpoint release").String())}}, nil
		}
	}
	return nil, nil
}

func installationTestPlugin(t testing.TB, revision string) *qimu.Plugin {
	t.Helper()
	definition, err := qimu.Define(qimu.Definition{Revision: qimu.Hash([]byte(revision))})
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func installationTestHost(t *testing.T, executor *InteractionExecutor, owners ...qimu.UseOwner) *qimu.Host {
	t.Helper()
	owners = append(owners, executor)
	host, err := qimu.NewHost(context.Background(), qimu.HostConfig{Admission: executor.installations, UseOwners: owners})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []identity.InstallationID{dependedInstallation, unrelatedInstallation} {
		if _, err := host.Install(t.Context(), qimu.InstallationConfig{ID: qimu.InstallationID(id.String()), Plugin: installationTestPlugin(t, id.String()), Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return host
}

func dependedRelease() []plugin.Dependency {
	return []plugin.Dependency{{InstallationID: dependedInstallation, Digest: testsupport.Digest("depended release")}}
}

func newInstallationTestExecutor(t *testing.T) *InteractionExecutor {
	t.Helper()
	return newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("installation admission invoked the model")
	}), InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Installations: dependedRelease(), Resources: dependedRelease()}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
}

func TestQuiescentInstallationChangeReadsOnlyDependencyProjections(t *testing.T) {
	for _, test := range []struct {
		name        string
		checkpoints pendingInstallationCheckpoints
		live        bool
		disable     bool
		refused     bool
	}{
		{name: "pending checkpoint dependency", checkpoints: pendingInstallationCheckpoints{dependedInstallation: true}, refused: true},
		{name: "live session dependency", checkpoints: pendingInstallationCheckpoints{}, live: true, refused: true},
		{name: "other installation's checkpoint", checkpoints: pendingInstallationCheckpoints{unrelatedInstallation: true}},
		{name: "revocation while in use", checkpoints: pendingInstallationCheckpoints{dependedInstallation: true}, live: true, disable: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := newInstallationTestExecutor(t)
			host := installationTestHost(t, executor, test.checkpoints)
			if test.live {
				ref, err := executor.StageRoot(t.Context(), interactionTestStart())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := executor.Release(context.Background(), ref); err != nil {
						t.Error(err)
					}
				})
			}
			var err error
			if test.disable {
				err = host.SetEnabled(t.Context(), qimu.InstallationID(dependedInstallation.String()), false)
			} else {
				err = host.Replace(t.Context(), qimu.InstallationID(dependedInstallation.String()), installationTestPlugin(t, "replacement"), nil)
			}
			if test.refused {
				if !errors.Is(err, qimu.ErrInUse) {
					t.Fatalf("unsafe replacement admitted: %v", err)
				}
			} else if err != nil {
				t.Fatalf("permitted change refused: %v", err)
			}
		})
	}
}

// A pending change must neither wait for a slow assembly nor, as a waiting
// writer, hold back an unrelated Run. The assembly it raced is refused at
// publication instead of becoming visible on a superseded release.
func TestInstallationChangeDoesNotWaitForAssembly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		executor := newInstallationTestExecutor(t)
		host := installationTestHost(t, executor)
		resolver := executor.config.ChatResolver
		entered, proceed := make(chan struct{}), make(chan struct{})
		var calls atomic.Int32
		executor.config.ChatResolver = interactionChatResolverFunc(func(ctx context.Context, selection modelchoice.Selection) (modeladapter.ResolvedChat, error) {
			if calls.Add(1) == 1 {
				close(entered)
				<-proceed
			}
			return resolver.ResolveChat(ctx, selection)
		})
		slow := make(chan error, 1)
		go func() {
			_, err := executor.StageRoot(t.Context(), interactionTestStart())
			slow <- err
		}()
		<-entered

		unrelated := make(chan error, 1)
		go func() {
			unrelated <- host.Replace(t.Context(), qimu.InstallationID(unrelatedInstallation.String()), installationTestPlugin(t, "unrelated replacement"), nil)
		}()
		synctest.Wait()
		select {
		case err := <-unrelated:
			if err != nil {
				t.Fatal(err)
			}
		default:
			close(proceed)
			t.Fatal("installation change waited for an unrelated assembly")
		}

		ref, err := executor.StageRoot(t.Context(), interactionTestStart())
		if err != nil {
			t.Fatalf("assembly behind a pending change: %v", err)
		}
		if err := host.Replace(t.Context(), qimu.InstallationID(dependedInstallation.String()), installationTestPlugin(t, "depended replacement"), nil); !errors.Is(err, qimu.ErrInUse) {
			t.Fatalf("change under a published dependency = %v", err)
		}
		if err := executor.Release(t.Context(), ref); err != nil {
			t.Fatal(err)
		}
		if err := host.Replace(t.Context(), qimu.InstallationID(dependedInstallation.String()), installationTestPlugin(t, "depended replacement"), nil); err != nil {
			t.Fatalf("change after release: %v", err)
		}

		close(proceed)
		if err := <-slow; !errors.Is(err, qimu.ErrStaleBinding) {
			t.Fatalf("assembly raced by a dependency change = %v, want stale refusal", err)
		}
		if live := executor.sessions.snapshot(); len(live) != 0 {
			t.Fatalf("refused assembly remained owned: %d sessions", len(live))
		}
	})
}

func TestWaitingCheckpointCarriesTheDependencyProjection(t *testing.T) {
	workspace := t.TempDir()
	executor := newObservedTestInteractionExecutor(t, &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "ask_call", Name: "ask", Arguments: `{}`}, 1, 1),
	}}, InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{identities: []domaintool.Ref{testsupport.A2ATool(t, "ask")}, manifest: toolset.Manifest{
			Installations: dependedRelease(), Resources: dependedRelease(), Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []run.InterruptKind{run.InterruptQuestion}
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	_, barrier := observeInteractionUntilWaiting(t, executor, ref, func() error {
		return executor.BeginRoot(t.Context(), ref)
	})
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	checkpoint := barrier.Checkpoint()
	if !slices.Equal(checkpoint.Dependencies(), dependedRelease()) {
		t.Fatalf("checkpoint dependencies = %+v", checkpoint.Dependencies())
	}
	if bytes.Contains(checkpoint.Payload, []byte(dependedInstallation.String())) {
		t.Fatal("continuation payload duplicates the dependency projection")
	}
}

func TestStagedModelTransitiveDependenciesBlockReplacementUntilRelease(t *testing.T) {
	leaf := plugin.Dependency{InstallationID: unrelatedInstallation, Digest: testsupport.Digest("transitive leaf")}
	owner := &contextCaptureComposition{t: t, dependencies: []plugin.Dependency{leaf}}
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		t.Fatal("dependency admission invoked a provider")
		return nil, nil
	}), InteractionExecutorConfig{CallComposition: owner})
	host := installationTestHost(t, executor)
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = executor.Release(context.Background(), ref) })
	if err := host.Replace(t.Context(), qimu.InstallationID(leaf.InstallationID.String()), installationTestPlugin(t, "leaf replacement"), nil); !errors.Is(err, qimu.ErrInUse) {
		t.Fatalf("transitive replacement admitted: %v", err)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if err := host.Replace(t.Context(), qimu.InstallationID(leaf.InstallationID.String()), installationTestPlugin(t, "leaf replacement"), nil); err != nil {
		t.Fatalf("released graph still held its leaf: %v", err)
	}
}
