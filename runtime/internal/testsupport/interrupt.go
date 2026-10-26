package testsupport

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

func QuestionAnswer(t testing.TB, answers [][]string) run.InterruptResolution {
	t.Helper()
	value, err := run.NewQuestionAnswer(answers)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func ApprovalAnswer(t testing.TB, answer approval.Resolution) run.InterruptResolution {
	t.Helper()
	value, err := run.NewApprovalAnswer(answer)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
