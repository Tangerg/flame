package testsupport

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
)

func QuestionAnswer(t testing.TB, answers [][]string) interrupt.Resolution {
	t.Helper()
	value, err := interrupt.NewQuestionAnswer(answers)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func ApprovalAnswer(t testing.TB, answer approval.Resolution) interrupt.Resolution {
	t.Helper()
	value, err := interrupt.NewApprovalAnswer(answer)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
