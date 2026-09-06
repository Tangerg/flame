package delivery

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	apphooks "github.com/Tangerg/flame/runtime/internal/application/integration/hooks"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domainhooks "github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/Tangerg/flame/runtime/protocol"
)

// fakeHookTrust records the workspace coordinator's trust calls (Trust/Untrust)
// so the hooks delivery handler can be tested against a wired trust store.
type fakeHookTrust struct {
	projectRoot string
	trusted     bool
	calls       int
}

func (f *fakeHookTrust) Trust(_ context.Context, projectRoot string) error {
	f.projectRoot = projectRoot
	f.trusted = true
	f.calls++
	return nil
}

func (f *fakeHookTrust) Untrust(_ context.Context, projectRoot string) error {
	f.projectRoot = projectRoot
	f.trusted = false
	f.calls++
	return nil
}

func handlerWithHookTrust(trust workspaceapp.HookTrustStore) *Handler {
	return newWorkspaceHandlerWithConfig("", workspaceTestConfig{Trust: trust})
}

func TestSetHookTrustCanonicalizesProjectRoot(t *testing.T) {
	trust := &fakeHookTrust{}
	s := handlerWithHookTrust(trust)
	projectRoot := t.TempDir()

	err := s.SetHookTrust(context.Background(), protocol.SetHookTrustRequest{
		ProjectRoot: projectRoot,
		Trusted:     true,
	})
	if err != nil {
		t.Fatalf("setTrust: %v", err)
	}
	if trust.calls != 1 || trust.projectRoot != canonicalWorkspacePath(t, projectRoot) || !trust.trusted {
		t.Fatalf("trusted root=%q trusted=%v calls=%d, want %q true 1", trust.projectRoot, trust.trusted, trust.calls, canonicalWorkspacePath(t, projectRoot))
	}
}

func TestSetHookTrustRejectsUnavailableProjectRoot(t *testing.T) {
	trust := &fakeHookTrust{}
	s := handlerWithHookTrust(trust)
	missing := filepath.Join(t.TempDir(), "missing")

	err := s.SetHookTrust(context.Background(), protocol.SetHookTrustRequest{
		ProjectRoot: missing,
		Trusted:     true,
	})
	if !errors.Is(err, protocol.ErrWorkspaceUnavailable) {
		t.Fatalf("setTrust err = %v, want ErrWorkspaceUnavailable", err)
	}
	if trust.calls != 0 {
		t.Fatalf("trust store calls = %d, want 0", trust.calls)
	}
}

type failingHookInspector struct{ err error }

func (f failingHookInspector) Inspect(context.Context, string) (apphooks.Inspection, error) {
	return apphooks.Inspection{}, f.err
}

type staticHookInspector struct{ inspection apphooks.Inspection }

func (s staticHookInspector) Inspect(context.Context, string) (apphooks.Inspection, error) {
	return s.inspection, nil
}

func TestListHooksPreservesCompleteHookDefinition(t *testing.T) {
	root := t.TempDir()
	projectRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	s := newWorkspaceHandlerWithConfig(root, workspaceTestConfig{Hooks: staticHookInspector{inspection: apphooks.Inspection{
		ProjectRoot: projectRoot,
		Hooks: []domainhooks.Hook{{
			Event: domainhooks.SubagentStart, Command: "audit", TimeoutMillis: 2500,
			Scope: domainhooks.ScopeGlobal, Source: "/home/user/.flame/hooks.json",
		}},
	}}})

	result, err := s.ListHooks(t.Context(), protocol.ListHooksRequest{})
	if err != nil {
		t.Fatalf("ListHooks: %v", err)
	}
	if len(result.Hooks) != 1 {
		t.Fatalf("hooks = %+v, want one", result.Hooks)
	}
	hook := result.Hooks[0]
	if hook.Event != protocol.HookEventSubagentStart || hook.TimeoutMillis != 2500 || !hook.Active {
		t.Fatalf("hook = %+v, want complete active subagent hook", hook)
	}
}

func TestListHooksPreservesInspectionFailure(t *testing.T) {
	wantErr := errors.New("hook trust unavailable")
	root := t.TempDir()
	s := newWorkspaceHandlerWithConfig(root, workspaceTestConfig{Hooks: failingHookInspector{err: wantErr}})

	if _, err := s.ListHooks(context.Background(), protocol.ListHooksRequest{}); !errors.Is(err, wantErr) {
		t.Fatalf("ListHooks error = %v, want %v", err, wantErr)
	}
}

type emptyHookInspector struct{}

func (emptyHookInspector) Inspect(_ context.Context, cwd string) (apphooks.Inspection, error) {
	return apphooks.Inspection{ProjectRoot: cwd}, nil
}
