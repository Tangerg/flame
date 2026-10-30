package toolset

import (
	"context"
	"strings"
	"testing"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/sandbox"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// The Runtime exposes two ways to change a file and no more. They are one
// vocabulary: same guard stack, same read-before-write stamp, and a path the
// guards can always name. edit carries that path as an argument; apply_patch
// declares it from the patch text and batches several at once.
func TestResolverRegistersTheMutationVocabulary(t *testing.T) {
	built, err := Build(BuildConfig{Lifetime: t.Context(), UserHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	manifest, err := built.Resolver.Manifest(attachedRun(t), domaintool.GroupRoot, nil)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	t.Cleanup(func() {
		if err := manifest.Close(); err != nil {
			t.Error(err)
		}
	})
	names := definitionNames(manifestTools(manifest))
	if !names[string(builtincontract.Edit)] || !names[string(builtincontract.ApplyPatch)] {
		t.Fatalf("mutation vocabulary = %v, want edit and apply_patch", names)
	}
	// write stays out: apply_patch already creates and deletes files, so a third
	// way to change one would be a second vocabulary for the same fact.
	if names["write"] {
		t.Fatalf("mutation vocabulary = %v, want no whole-file write", names)
	}
}

func TestResolverInitialManifestSeparatesDirectAndDeferredCapabilities(t *testing.T) {
	named := func(name string) toolcontract.Tool {
		t.Helper()
		candidate, err := toolcontract.NewFunc(
			toolcontract.FuncConfig{Name: name, Description: "test " + name},
			func(context.Context, struct{}) (string, error) { return "ok", nil },
		)
		if err != nil {
			t.Fatalf("build %s: %v", name, err)
		}
		return candidate
	}
	resolver, err := newResolver(resolverDeps{
		Shell:        []toolcontract.Tool{named(string(builtincontract.Shell))},
		AskUser:      named(string(builtincontract.AskUser)),
		EnterPlan:    named(string(builtincontract.EnterPlanMode)),
		ExitPlan:     named(string(builtincontract.ExitPlanMode)),
		Plan:         named(string(builtincontract.SetPlan)),
		ToolResult:   named(string(builtincontract.ReadToolResult)),
		GoalGet:      named(string(builtincontract.GetGoal)),
		ProposeSkill: named(string(builtincontract.ProposeSkill)),
		ReadTracker:  newReadTracker(),
	})
	if err != nil {
		t.Fatalf("newResolver: %v", err)
	}
	resolver.SetMCPTools([]toolcontract.Tool{
		mcpToolStub{name: "linear_create_issue", server: "linear", remote: "create_issue"},
	}, nil)
	manifest, err := resolver.Manifest(attachedRun(t), domaintool.GroupRoot, nil)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	t.Cleanup(func() {
		if err := manifest.Close(); err != nil {
			t.Error(err)
		}
	})
	registered := definitionNames(manifestTools(manifest))
	advertised := definitionNames(manifest.Visible)
	for _, name := range []string{
		string(builtincontract.Read), string(builtincontract.Glob), string(builtincontract.Grep), string(builtincontract.ApplyPatch), string(builtincontract.Shell), string(builtincontract.AskUser),
		string(builtincontract.EnterPlanMode), string(builtincontract.ExitPlanMode), string(builtincontract.SetPlan), string(builtincontract.ReadToolResult),
		string(builtincontract.GetGoal), string(builtincontract.SearchTools),
	} {
		if !advertised[name] {
			t.Errorf("direct tool %q missing from initial manifest: %v", name, advertised)
		}
	}
	for _, name := range []string{
		"linear_create_issue",
		"propose_skill",
	} {
		if !registered[name] {
			t.Errorf("deferred tool %q missing from Run registry: %v", name, registered)
		}
		if advertised[name] {
			t.Errorf("deferred tool %q leaked into initial manifest: %v", name, advertised)
		}
	}
}

func manifestTools(manifest Manifest) []toolcontract.Tool {
	return append(append([]toolcontract.Tool(nil), manifest.Visible...), manifest.Deferred...)
}

func TestBuildRequiresExplicitProcessPaths(t *testing.T) {
	validPaths := BuildConfig{Lifetime: t.Context(), UserHome: t.TempDir()}
	validPaths.Lifetime = nil
	if _, err := Build(validPaths); err == nil {
		t.Fatal("Build accepted a nil process lifetime")
	}
	if _, err := Build(BuildConfig{Lifetime: t.Context()}); err == nil {
		t.Fatal("Build accepted an empty user home")
	}
	if _, err := Build(BuildConfig{Lifetime: t.Context(), UserHome: "relative"}); err == nil {
		t.Fatal("Build accepted a relative user home")
	}
}

// Isolated sessions jail their shell without the global sandbox opt-in, so a
// confiner configuration error must reach the isolated command rather than
// read as a host with no sandbox backend.
func TestBuildKeepsTheConfinerCauseForIsolatedShells(t *testing.T) {
	config := BuildConfig{
		Lifetime:             t.Context(),
		UserHome:             t.TempDir(),
		SandboxReadOnlyPaths: []string{"relative-toolchain"},
	}
	_, cause := sandbox.NewConfiner(config.UserHome, config.SandboxReadOnlyPaths)
	if cause == nil {
		t.Fatal("NewConfiner accepted a relative read-only path")
	}
	built, err := Build(config)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)

	_, err = built.Shells.Launch(t.Context(), "session", t.TempDir(), "true", exec.Timeout{}, true)
	if err == nil || !strings.Contains(err.Error(), cause.Error()) {
		t.Fatalf("isolated launch error = %v, want cause %q", err, cause)
	}
}
