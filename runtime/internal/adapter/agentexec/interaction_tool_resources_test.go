package agentexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type capturedResourceManifest struct {
	ctx      context.Context
	manifest toolset.Manifest
}

type resourceManifestResolver struct {
	resolver *toolset.Resolver
	failure  string
	captured []capturedResourceManifest
}

func (r *resourceManifestResolver) Manifest(ctx context.Context, group domaintool.Group) (toolset.Manifest, error) {
	if r.failure == "delegated resolution" && group == domaintool.GroupDelegated {
		return toolset.Manifest{}, errors.New("delegated resolution failed")
	}
	manifest, err := r.resolver.Manifest(ctx, group)
	if err != nil {
		return toolset.Manifest{}, err
	}
	r.captured = append(r.captured, capturedResourceManifest{ctx: ctx, manifest: manifest})
	if r.failure == "manifest validation" {
		manifest.Visible = append(manifest.Visible, manifest.Visible[0])
	}
	return manifest, nil
}

func TestInteractionReleasesScopedFilesystemTools(t *testing.T) {
	for _, lifecycle := range []string{"release", "completion", "shutdown", "manifest validation", "delegated resolution"} {
		t.Run(lifecycle, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("note"), 0o600); err != nil {
				t.Fatal(err)
			}
			built, err := toolset.Build(t.Context(), toolset.BuildConfig{Lifetime: t.Context(), DefaultCWD: root, UserHome: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				for _, close := range built.Closers {
					if err := close(); err != nil {
						t.Error(err)
					}
				}
			})
			resolver := &resourceManifestResolver{resolver: built.Resolver, failure: lifecycle}
			model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				if lifecycle != "completion" {
					t.Fatal("staging must not invoke the model")
				}
				return interactionTextResponse("done"), nil
			})
			executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
				ToolResolver: resolver, ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
			})
			start := interactionTestStart()
			start.CWD, start.WorkspaceCWD = root, root
			start.ChildRunAdmissionEnabled = true
			if lifecycle == "completion" {
				runInteractionHarness(t.Context(), t, executor, start, nil)
			} else {
				ref, err := executor.StageRoot(t.Context(), start)
				wantFailure := lifecycle == "manifest validation" || lifecycle == "delegated resolution"
				if (err != nil) != wantFailure {
					t.Fatalf("StageRoot = %v, lifecycle %q", err, lifecycle)
				}
				if err == nil {
					if lifecycle == "shutdown" {
						executor.BeginShutdown()
						err = executor.AwaitShutdown(t.Context())
					} else {
						err = executor.Release(t.Context(), ref)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			if len(resolver.captured) == 0 {
				t.Fatal("no filesystem capability was created")
			}
			for _, captured := range resolver.captured {
				for _, executable := range captured.manifest.Visible {
					if executable.Definition().Name != domaintool.Read {
						continue
					}
					binding, err := toolcontract.Bind(executable)
					if err != nil {
						t.Fatal(err)
					}
					invocation, err := binding.Contract().Prepare(chat.ToolCall{ID: "read", Name: domaintool.Read, Arguments: `{"path":"note.txt"}`})
					if err != nil {
						t.Fatal(err)
					}
					_, err = binding.Call(captured.ctx, invocation)
					failure, ok := errors.AsType[*toolcontract.Failure](err)
					if !ok || !errors.Is(failure.Cause(), os.ErrClosed) {
						t.Errorf("filesystem authority survived execution release: %v", err)
					}
				}
			}
		})
	}
}
