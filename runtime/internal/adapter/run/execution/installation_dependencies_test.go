package execution

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

var (
	dependedInstallation  = mustInstallationID("940ac827-b431-455b-af4b-e3a170bcfda0")
	unrelatedInstallation = mustInstallationID("1d7f3c2e-6a1b-4f0e-9c3d-2b8a7e5f4c10")
)

func mustInstallationID(text string) resourceid.InstallationID {
	id, err := resourceid.ParseInstallation(text)
	if err != nil {
		panic(err)
	}
	return id
}

type pendingInstallationCheckpoints map[resourceid.InstallationID]bool

func (p pendingInstallationCheckpoints) PendingCheckpointDependencies(context.Context) ([]plugin.Dependency, error) {
	var dependencies []plugin.Dependency
	for id, pending := range p {
		if pending {
			dependencies = append(dependencies, plugin.Dependency{InstallationID: id, Digest: testsupport.Digest("checkpoint release")})
		}
	}
	return plugin.CompactDependencies(dependencies), nil
}

func dependedRelease() []plugin.Dependency {
	return []plugin.Dependency{{InstallationID: dependedInstallation, Digest: testsupport.Digest("depended release")}}
}

func newInstallationTestExecutor(t *testing.T, checkpoints InstallationCheckpoints) *InteractionExecutor {
	t.Helper()
	return newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("installation admission invoked the model")
	}), InteractionExecutorConfig{
		InstallationCheckpoints: checkpoints,
		ToolResolver:            staticInteractionTools{manifest: toolset.Manifest{Installations: dependedRelease()}},
		ToolInterpreter:         testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
}

func TestQuiescentInstallationChangeReadsOnlyDependencyProjections(t *testing.T) {
	for _, test := range []struct {
		name        string
		checkpoints pendingInstallationCheckpoints
		live        bool
		admission   plugins.ChangeAdmission
		refused     bool
	}{
		{name: "pending checkpoint dependency", checkpoints: pendingInstallationCheckpoints{dependedInstallation: true}, admission: plugins.RequireQuiescent, refused: true},
		{name: "live session dependency", live: true, admission: plugins.RequireQuiescent, refused: true},
		{name: "other installation's checkpoint", checkpoints: pendingInstallationCheckpoints{unrelatedInstallation: true}, admission: plugins.RequireQuiescent},
		{name: "revocation while in use", checkpoints: pendingInstallationCheckpoints{dependedInstallation: true}, live: true, admission: plugins.AllowInUse},
	} {
		t.Run(test.name, func(t *testing.T) {
			executor := newInstallationTestExecutor(t, test.checkpoints)
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
			changed := false
			err := executor.ChangeInstallation(t.Context(), dependedInstallation, test.admission, func([]plugin.Dependency) error {
				changed = true
				return nil
			})
			if test.refused {
				if !errors.Is(err, plugin.ErrInUse) || changed {
					t.Fatalf("unsafe change admitted: %v, changed=%v", err, changed)
				}
			} else if err != nil || !changed {
				t.Fatalf("permitted change refused: %v, changed=%v", err, changed)
			}
		})
	}
}

// A pending change must neither wait for a slow assembly nor, as a waiting
// writer, hold back an unrelated Run. The assembly it raced is refused at
// publication instead of becoming visible on a superseded release.
func TestInstallationChangeDoesNotWaitForAssembly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		executor := newInstallationTestExecutor(t, pendingInstallationCheckpoints{})
		resolver := executor.config.ChatResolver
		entered, proceed := make(chan struct{}), make(chan struct{})
		var calls atomic.Int32
		executor.config.ChatResolver = interactionChatResolverFunc(func(ctx context.Context, selection modelref.Selection) (modeladapter.ResolvedChat, error) {
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
			unrelated <- executor.ChangeInstallation(t.Context(), unrelatedInstallation, plugins.RequireQuiescent, func([]plugin.Dependency) error { return nil })
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
		if err := executor.ChangeInstallation(t.Context(), dependedInstallation, plugins.RequireQuiescent, func([]plugin.Dependency) error { return nil }); !errors.Is(err, plugin.ErrInUse) {
			t.Fatalf("change under a published dependency = %v", err)
		}
		if err := executor.Release(t.Context(), ref); err != nil {
			t.Fatal(err)
		}
		if err := executor.ChangeInstallation(t.Context(), dependedInstallation, plugins.RequireQuiescent, func([]plugin.Dependency) error { return nil }); err != nil {
			t.Fatalf("change after release: %v", err)
		}

		close(proceed)
		if err := <-slow; !errors.Is(err, runs.ErrInstallationChanged) {
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
			Installations: dependedRelease(), Visible: []toolcontract.Tool{newQuestionCheckpointTool(t)},
		}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	start := interactionTestStart()
	start.CWD, start.WorkspaceCWD = workspace, workspace
	start.InterruptKinds = []interrupt.Kind{interrupt.Question}
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
	if !slices.Equal(checkpoint.Installations, dependedRelease()) {
		t.Fatalf("checkpoint dependencies = %+v", checkpoint.Installations)
	}
	if bytes.Contains(checkpoint.Payload, []byte(dependedInstallation.String())) {
		t.Fatal("continuation payload duplicates the dependency projection")
	}
}

// A change holds the serialization point across durable storage. Assembly
// registration must not queue behind it, and a party waiting for the point must
// still honor its own cancellation.
func TestInstallationAdmissionNeverQueuesAssemblyOrIgnoresCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		admission := newInstallationAdmission()
		leave, err := admission.enter(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer leave()

		registered := make(chan struct{})
		go func() {
			_, finish := admission.begin()
			finish()
			close(registered)
		}()
		synctest.Wait()
		select {
		case <-registered:
		default:
			t.Fatal("assembly registration waited behind a held change")
		}

		cause := errors.New("change abandoned")
		ctx, cancel := context.WithCancelCause(t.Context())
		waited := make(chan error, 1)
		go func() {
			_, err := admission.enter(ctx)
			waited <- err
		}()
		synctest.Wait()
		cancel(cause)
		synctest.Wait()
		if err := <-waited; !errors.Is(err, cause) {
			t.Fatalf("waiting change = %v, want its own cancellation", err)
		}
	})
}
