package maintenance

import (
	"context"
	"errors"
	"testing"

	execution "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
)

type maintenanceFixture struct {
	skillsErr, memoryErr error
	inputs               []execution.RunMaintenanceInput
}

func (f *maintenanceFixture) MaintainSkills(_ context.Context, input execution.RunMaintenanceInput) error {
	f.inputs = append(f.inputs, input)
	return f.skillsErr
}
func (f *maintenanceFixture) MaintainMemory(_ context.Context, input execution.RunMaintenanceInput) error {
	f.inputs = append(f.inputs, input)
	return f.memoryErr
}

func TestPipelineContinuesIndependentMaintenanceAndPreservesFailures(t *testing.T) {
	f := &maintenanceFixture{skillsErr: errors.New("skill learning failed"), memoryErr: errors.New("memory failed")}
	pipeline, err := NewPipeline(f, f)
	if err != nil {
		t.Fatal(err)
	}
	input := execution.RunMaintenanceInput{SessionID: "ses_completed", WorkspaceCWD: t.TempDir(), ToolCalls: 8, DurableContextCompacted: true}
	result := pipeline.Maintain(t.Context(), input)
	if failure := errors.Join(result.Errors...); !errors.Is(failure, f.skillsErr) || !errors.Is(failure, f.memoryErr) {
		t.Fatalf("maintenance failures = %v", failure)
	}
	if len(f.inputs) != 2 || f.inputs[0] != input || f.inputs[1] != input {
		t.Fatalf("completed facts = %+v", f.inputs)
	}
}

func TestPipelineRequiresCompleteMaintenanceConsumers(t *testing.T) {
	valid := &maintenanceFixture{}
	var absent *maintenanceFixture
	if _, err := NewPipeline(absent, valid); err == nil {
		t.Fatal("typed-nil Memory accepted")
	}
	if _, err := NewPipeline(valid, absent); err == nil {
		t.Fatal("typed-nil Skill learning accepted")
	}
}
