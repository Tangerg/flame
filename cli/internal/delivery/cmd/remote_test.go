package cmd

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/cli/internal/application/settings"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func TestRuntimeEndpointPreferenceAndCredentialBoundary(t *testing.T) {
	local := t.TempDir()
	writeCommandFixture(t, filepath.Join(local, ".flame.yaml"), []byte("runtime:\n  endpoint: https://file.example/flame\n"))
	t.Setenv("FLAME_CLI_RUNTIME_ENDPOINT", "https://environment.example/flame")
	t.Setenv("FLAME_RUNTIME_TOKEN", "secret-runtime-token")
	for _, test := range []struct {
		flags []string
		want  string
	}{
		{want: "https://environment.example/flame"},
		{flags: []string{"--runtime-url", "https://flag.example/flame"}, want: "https://flag.example/flame"},
		{flags: []string{"--runtime-url="}, want: ""},
	} {
		args := append([]string{"-C", local}, test.flags...)
		args = append(args, "config", "show")
		out, _, err := executeCommand(t, instantRuntime(), "", args...)
		if err != nil {
			t.Fatal(err)
		}
		var configured settings.Config
		if err := json.Unmarshal([]byte(out), &configured); err != nil {
			t.Fatal(err)
		}
		if configured.Runtime.Endpoint != test.want || strings.Contains(out, "secret-runtime-token") {
			t.Fatalf("effective Runtime preferences = %q", out)
		}
	}
}

func TestRemoteTerminalSeparatesTargetWorkspaceFromLocalAuthoring(t *testing.T) {
	local, state := t.TempDir(), t.TempDir()
	canonical, err := filepath.EvalSymlinks(local)
	if err != nil {
		t.Fatal(err)
	}
	var requests []TerminalRequest
	for index, endpoint := range []string{"https://one.example", "https://two.example"} {
		root := NewRoot(Dependencies{
			StateDirectory: state,
			StartTerminal: func(_ context.Context, request TerminalRequest) error {
				requests = append(requests, request)
				return nil
			},
		})
		args := []string{"--runtime-url", endpoint, "-C", local}
		if index == 0 {
			args = append(args, "--workspace", `C:\remote\project`)
		}
		root.SetArgs(append(args, "inspect"))
		if err := root.ExecuteContext(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for _, request := range requests {
		if request.LocalDirectory != canonical || request.StateDirectory == state {
			t.Fatalf("remote terminal request = %+v", request)
		}
	}
	if requests[0].Workspace != `C:\remote\project` || requests[1].Workspace != "" {
		t.Fatalf("explicit/default Runtime workspace = %q, %q", requests[0].Workspace, requests[1].Workspace)
	}
	if requests[0].StateDirectory == requests[1].StateDirectory {
		t.Fatal("distinct Runtime targets share authoring state")
	}
}

func TestRemoteRunAttachesLocalBytesToAnExistingRemoteSession(t *testing.T) {
	local := t.TempDir()
	writeCommandFixture(t, filepath.Join(local, "notes.txt"), []byte("local attachment"))
	runtime := instantRuntime()
	runtime.Script = shortCompletedScript
	created, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: `C:\remote\project`})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = executeCommand(t, runtime, "", "--runtime-url", "https://runtime.example", "-C", local,
		"run", "--session", created.ID, "--file", "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.GetSession(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	prompt := userPromptBlock(t, snapshot.Transcript)
	canonical, err := filepath.EvalSymlinks(filepath.Join(local, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt.Attachments) != 1 || prompt.Attachments[0].Path != canonical {
		t.Fatalf("local attachment = %+v", prompt.Attachments)
	}
}

func TestRemoteFileCompletionDoesNotOpenRuntime(t *testing.T) {
	local := t.TempDir()
	writeCommandFixture(t, filepath.Join(local, "notes.txt"), []byte("notes"))
	root := NewRoot(Dependencies{OpenRuntime: func(context.Context, string) (Runtime, *runtimebinding.Profile, error) {
		t.Fatal("local file completion opened Runtime")
		return nil, nil, nil
	}})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"__complete", "--runtime-url", "https://runtime.example", "-C", local,
		"run", "--session", "ses_remote", "--file", "notes"})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "notes.txt") {
		t.Fatalf("file completion = %q", out.String())
	}
}

func TestDynamicCompletionLoadsTheConfiguredRuntimeTarget(t *testing.T) {
	local := t.TempDir()
	writeCommandFixture(t, filepath.Join(local, ".flame.yaml"), []byte("runtime:\n  endpoint: https://configured.example/flame\n"))
	var target string
	root := NewRoot(Dependencies{OpenRuntime: func(_ context.Context, endpoint string) (Runtime, *runtimebinding.Profile, error) {
		target = endpoint
		return instantRuntime(), nil, nil
	}})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"__complete", "-C", local, "sessions", "show", ""})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if target != "https://configured.example/flame" || !strings.Contains(out.String(), "ses_demo_") {
		t.Fatalf("completion target/output = %q, %q", target, out.String())
	}
}

func TestExplicitRemoteFailureDoesNotOpenAnEmbeddedRuntime(t *testing.T) {
	want := errors.New("remote endpoint unavailable")
	var endpoints []string
	root := NewRoot(Dependencies{OpenRuntime: func(_ context.Context, endpoint string) (Runtime, *runtimebinding.Profile, error) {
		endpoints = append(endpoints, endpoint)
		return nil, nil, want
	}})
	root.SetArgs([]string{"--runtime-url", "https://offline.example", "sessions", "ls"})
	if err := root.ExecuteContext(t.Context()); !errors.Is(err, want) {
		t.Fatalf("remote failure = %v", err)
	}
	if len(endpoints) != 1 || endpoints[0] != "https://offline.example" {
		t.Fatalf("Runtime targets opened = %v", endpoints)
	}
}

func TestRemoteWorkspaceDoesNotSelectLocalProjectConfiguration(t *testing.T) {
	local := t.TempDir()
	writeCommandFixture(t, filepath.Join(local, ".flame.yaml"), []byte("ui:\n  mouse: false\n"))
	remote := t.TempDir()
	writeCommandFixture(t, filepath.Join(remote, ".flame.yaml"), []byte("invalid: target configuration must not be read\n"))
	var request TerminalRequest
	root := NewRoot(Dependencies{StartTerminal: func(_ context.Context, value TerminalRequest) error {
		request = value
		return nil
	}})
	root.SetArgs([]string{"--runtime-url", "https://runtime.example", "-C", local, "--workspace", remote})
	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if request.Settings.UI.Mouse || request.Workspace != remote {
		t.Fatalf("Runtime workspace changed CLI preferences = %+v", request)
	}
}
