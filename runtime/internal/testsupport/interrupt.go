package testsupport

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func QuestionAnswer(t testing.TB, answers [][]string) run.InterruptResolution {
	t.Helper()
	value, err := run.NewQuestionAnswer(answers)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
