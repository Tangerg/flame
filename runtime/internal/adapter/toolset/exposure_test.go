package toolset

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset/codeintel"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
	"github.com/Tangerg/flame/runtime/internal/infra/process/sandbox"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func newTestCodeIntel(t *testing.T) *codeintel.Analyzer {
	t.Helper()
	analyzer, err := codeintel.New(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = analyzer.Close() })
	return analyzer
}

// The Runtime exposes two ways to change a file and no more. They are one
// vocabulary: same guard stack, same read-before-write stamp, and a path the
// guards can always name. edit carries that path as an argument; apply_patch
// declares it from the patch text and batches several at once.
func TestResolverRegistersTheMutationVocabulary(t *testing.T) {
	built, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), UserHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	manifest, err := built.Resolver.Manifest(attachedRun(t), domaintool.GroupRoot)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	t.Cleanup(func() {
		if err := manifest.Close(); err != nil {
			t.Error(err)
		}
	})
	names := definitionNames(manifestTools(manifest))
	if !names[string(domaintool.Edit)] || !names[string(domaintool.ApplyPatch)] {
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
		if name == "remote_agent" {
			ref, err := domaintool.A2A(name)
			if err != nil {
				t.Fatal(err)
			}
			identified, err := WithIdentity(candidate, ref, testsupport.ToolFingerprint(ref))
			if err != nil {
				t.Fatal(err)
			}
			return identified
		}
		return candidate
	}
	analyzer := newTestCodeIntel(t)
	resolver, err := newResolver(resolverDeps{
		Online:    []toolcontract.Tool{named(string(domaintool.WebFetch))},
		A2A:       []toolcontract.Tool{named("remote_agent")},
		LSP:       []toolcontract.Tool{named(string(domaintool.LSP))},
		Shell:     []toolcontract.Tool{named(string(domaintool.Shell))},
		AskUser:   named(string(domaintool.AskUser)),
		EnterPlan: named(string(domaintool.EnterPlanMode)),
		ExitPlan:  named(string(domaintool.ExitPlanMode)),
		Plan:      named(string(domaintool.SetPlan)),
		ScheduleTools: []toolcontract.Tool{
			named(string(domaintool.ListSchedules)), named(string(domaintool.CreateSchedule)), named(string(domaintool.DeleteSchedule)),
		},
		ToolResult:        named(string(domaintool.ReadToolResult)),
		AgentMemorySearch: named(string(domaintool.SearchMemory)),
		GoalGet:           named(string(domaintool.GetGoal)),
		ProposeSkill:      named(string(domaintool.ProposeSkill)),
		CodeIntel:         analyzer,
		ReadTracker:       newReadTracker(),
	})
	if err != nil {
		t.Fatalf("newResolver: %v", err)
	}
	resolver.SetMCPTools([]toolcontract.Tool{
		mcpToolStub{name: "linear_create_issue", server: "linear", remote: "create_issue"},
	})
	manifest, err := resolver.Manifest(attachedRun(t), domaintool.GroupRoot)
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
		string(domaintool.Read), string(domaintool.Glob), string(domaintool.Grep), string(domaintool.ApplyPatch), string(domaintool.Shell), string(domaintool.AskUser),
		string(domaintool.EnterPlanMode), string(domaintool.ExitPlanMode), string(domaintool.SetPlan), string(domaintool.ReadToolResult),
		string(domaintool.GetGoal), string(domaintool.SearchTools),
	} {
		if !advertised[name] {
			t.Errorf("direct tool %q missing from initial manifest: %v", name, advertised)
		}
	}
	for _, name := range []string{
		"web_fetch", "remote_agent", "lsp", "linear_create_issue", "list_schedules",
		"create_schedule", "delete_schedule",
		"search_memory", "propose_skill",
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
	var missingContext context.Context
	if _, err := Build(missingContext, validPaths); err == nil {
		t.Fatal("Build accepted a nil startup context")
	}
	validPaths.Lifetime = nil
	if _, err := Build(t.Context(), validPaths); err == nil {
		t.Fatal("Build accepted a nil process lifetime")
	}
	if _, err := Build(t.Context(), BuildConfig{Lifetime: t.Context()}); err == nil {
		t.Fatal("Build accepted an empty user home")
	}
	if _, err := Build(t.Context(), BuildConfig{Lifetime: t.Context(), UserHome: "relative"}); err == nil {
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
	built, err := Build(t.Context(), config)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)

	_, err = built.Shells.Launch(t.Context(), "session", t.TempDir(), "true", exec.Timeout{}, true)
	if err == nil || !strings.Contains(err.Error(), cause.Error()) {
		t.Fatalf("isolated launch error = %v, want cause %q", err, cause)
	}
}
