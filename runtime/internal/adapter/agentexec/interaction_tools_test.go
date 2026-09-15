package agentexec

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func TestToolHookDecisionOwnsArguments(t *testing.T) {
	arguments, err := tool.ParseArguments(`{"path":"approved"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := arguments
	decision := AllowToolHook(true, &arguments)
	arguments = tool.Arguments{}
	got, present := decision.EffectiveArguments()
	if !present || !got.Equal(want) || !decision.RequiresApproval() {
		t.Fatalf("decision changed after caller reassigned arguments: %v, %v", got, present)
	}
	if _, present := AllowToolHook(false, nil).EffectiveArguments(); present {
		t.Fatal("absent rewrite became an empty-argument rewrite")
	}
	if got, present := AllowToolHook(false, &arguments).EffectiveArguments(); !present || !got.Equal(arguments) {
		t.Fatal("explicit empty-argument rewrite was lost")
	}
}
