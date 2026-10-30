package toolset

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// rejectedArguments are schema-valid calls each tool refuses on its own terms.
// A tool missing from this table fails the guard rather than being skipped: the
// question it answers has to be asked of every tool the Runtime exposes.
var rejectedArguments = map[string]string{
	string(builtincontract.ApplyPatch):        `{"patch":"this is not a unified diff"}`,
	string(builtincontract.Edit):              `{"path":"seed.txt","old_string":"absent from the file","new_string":"x"}`,
	string(builtincontract.Glob):              `{"pattern":"["}`,
	string(builtincontract.Grep):              `{"pattern":"("}`,
	string(builtincontract.LSP):               `{"operation":"definition"}`,
	string(builtincontract.Read):              `{"path":"no/such/file.txt"}`,
	string(builtincontract.ReadShellOutput):   `{"shell_id":"shell_x","timeout_millis":10}`,
	string(builtincontract.SearchTools):       `{"query":"   "}`,
	string(builtincontract.Shell):             `{"command":"true","description":"Probe","run_in_background":true,"auto_background_after_seconds":5}`,
	string(builtincontract.StopShell):         `{"shell_id":"shell_missing"}`,
	string(builtincontract.ListSkills):        `{}`,
	string(builtincontract.LoadSkill):         `{"name":"no-such-skill"}`,
	string(builtincontract.ReadSkillResource): `{"name":"no-such-skill","path":"no/such/resource.md"}`,
	// ask_user's refusals are all schema-level, and execution adapter's own invoke
	// settles a Prepare failure as a definite failure — a layer above this one.
	string(builtincontract.AskUser): "",
}

// The Host reads an unclassified Tool error as an operation whose durable
// outcome it cannot prove, and settles the whole Run tree — root and every
// delegated child — as lost. A call the model can simply write again must
// therefore never return one. Tools that may have reached the network or
// spawned work are deliberately absent from the table above: for those,
// "outcome unknown" is the true answer.
func TestRejectedCallsDoNotCostTheRun(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	built, err := Build(t.Context(), BuildConfig{
		Lifetime: t.Context(), UserHome: t.TempDir(),
		SkillsUserDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	closeBuiltToolset(t, built)
	manifest, err := built.Resolver.Manifest(attachedRun(t), domaintool.GroupRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manifest.Close(); err != nil {
			t.Error(err)
		}
	})

	for _, exposed := range manifestTools(manifest) {
		name := exposed.Definition().Name
		arguments, listed := rejectedArguments[name]
		if !listed {
			t.Errorf("tool %q has no rejected-call probe: decide whether its refusals are "+
				"definite (add one) or may have had an external effect (say so here)", name)
			continue
		}
		if arguments == "" {
			continue
		}
		_, callErr := callTextTool(t.Context(), exposed, arguments)
		if callErr == nil {
			continue // Query tools may answer normally, including empty results.
		}
		var failure *toolcontract.Failure
		if !errors.As(callErr, &failure) {
			t.Errorf("tool %q returned an unclassified error (%v); the Host loses the Run tree on those",
				name, callErr)
		}
	}
}
