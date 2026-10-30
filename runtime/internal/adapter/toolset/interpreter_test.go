package toolset

import (
	"context"
	"errors"
	"testing"
	"time"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestSemanticsSafetyClassFailsClosed(t *testing.T) {
	interpreter := Interpreter{}
	for _, test := range []struct {
		ref  tool.Ref
		want tool.SafetyClass
	}{
		{ref: testsupport.BuiltInTool(t, builtincontract.DelegateTask), want: tool.SafetyClassSafe},
		{ref: testsupport.BuiltInTool(t, builtincontract.ApplyPatch), want: tool.SafetyClassWrite},
		{ref: testsupport.BuiltInTool(t, builtincontract.Shell), want: tool.SafetyClassExec},
		{ref: testsupport.BuiltInTool(t, builtincontract.ReadShellOutput), want: tool.SafetyClassSafe},
		{ref: testsupport.BuiltInTool(t, builtincontract.StopShell), want: tool.SafetyClassExec},
		{ref: testsupport.BuiltInTool(t, builtincontract.HTTPRequest), want: tool.SafetyClassNetwork},
		{ref: tool.Ref{}, want: tool.SafetyClassExec},
		{ref: testsupport.A2ATool(t, "shell"), want: tool.SafetyClassExec},
	} {
		if got := interpreter.SafetyClass(test.ref); got != test.want {
			t.Errorf("SafetyClass(%q) = %q, want %q", test.ref, got, test.want)
		}
	}
}

func TestSemanticsApprovalSubject(t *testing.T) {
	interpreter := Interpreter{}
	for _, test := range []struct {
		name      builtincontract.Name
		arguments string
		want      string
		wantError bool
	}{
		{name: builtincontract.Shell, arguments: `{"command":"npm run build"}`, want: "npm run build"},
		{name: builtincontract.Read, arguments: `{"path":"go.mod"}`, want: "go.mod"},
		{name: builtincontract.ApplyPatch, arguments: `{"patch":"diff"}`},
		{name: builtincontract.Grep, arguments: `{"pattern":"foo"}`},
		{name: builtincontract.StopShell, arguments: `{"shell_id":"s1"}`},
		{name: builtincontract.Shell, arguments: `{"timeout_millis":5}`, wantError: true},
	} {
		arguments, err := tool.ParseArguments(test.arguments)
		if err != nil {
			t.Fatalf("ParseArguments(%q): %v", test.arguments, err)
		}
		got, err := interpreter.ApprovalSubject(testsupport.BuiltInTool(t, test.name), arguments)
		if test.wantError {
			if !errors.Is(err, tool.ErrInvalidArguments) {
				t.Errorf("ApprovalSubject(%q) error = %v, want invalid arguments", test.name, err)
			}
			continue
		}
		if err != nil || got != test.want {
			t.Errorf("ApprovalSubject(%q) = (%q, %v), want %q", test.name, got, err, test.want)
		}
	}
}

func TestSemanticsShellCommand(t *testing.T) {
	interpreter := Interpreter{}
	if got := interpreter.ShellCommand(testsupport.BuiltInTool(t, builtincontract.Shell), `{"command":"rm -rf /"}`); got != "rm -rf /" {
		t.Fatalf("ShellCommand = %q, want command", got)
	}
	if got := interpreter.ShellCommand(testsupport.BuiltInTool(t, builtincontract.ApplyPatch), `{"command":"rm -rf /"}`); got != "" {
		t.Fatalf("non-shell command = %q, want empty", got)
	}
}

func TestSemanticsDelegationUsesChildLifecyclePolicy(t *testing.T) {
	interpreter := Interpreter{}
	if interpreter.UsesStandardPolicy(testsupport.BuiltInTool(t, builtincontract.DelegateTask)) {
		t.Fatal("delegation entered ordinary tool-call policy")
	}
	for _, ref := range []tool.Ref{testsupport.BuiltInTool(t, builtincontract.Read), testsupport.A2ATool(t, "extension_tool")} {
		if !interpreter.UsesStandardPolicy(ref) {
			t.Errorf("ordinary tool %q bypassed standard policy", ref)
		}
	}
}

func TestSemanticsProjectsSuccessfulPlanReplacement(t *testing.T) {
	want, err := plan.Restore(plan.Snapshot{
		Revision: 7, UpdatedAt: time.Now(),
		Steps: []plan.Step{{Description: "verify", Status: plan.StatusInProgress}},
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := plan.CurrentOf(want)
	if err != nil {
		t.Fatal(err)
	}
	interpreter := NewInterpreter(fixedPlanState{state: current})
	event, err := interpreter.ProjectOutcome(t.Context(), "session_1", testsupport.BuiltInTool(t, builtincontract.SetPlan), true)
	if err != nil {
		t.Fatal(err)
	}
	updated, ok := event.(runs.PlanUpdated)
	if !ok || updated.State.Revision() != want.Revision() || len(updated.State.Steps()) != 1 {
		t.Fatalf("projected event = %#v, want PlanUpdated %#v", event, want)
	}
	for _, test := range []struct {
		name      builtincontract.Name
		succeeded bool
	}{
		{name: builtincontract.SetPlan},
		{name: builtincontract.Read, succeeded: true},
	} {
		event, err := interpreter.ProjectOutcome(t.Context(), "session_1", testsupport.BuiltInTool(t, test.name), test.succeeded)
		if err != nil || event != nil {
			t.Errorf("ProjectOutcome(%q, %t) = (%#v, %v), want no event", test.name, test.succeeded, event, err)
		}
	}
}

type fixedPlanState struct {
	state plan.Current
	err   error
}

func (f fixedPlanState) State(context.Context, string) (plan.Current, error) {
	return f.state, f.err
}
