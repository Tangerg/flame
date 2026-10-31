package run

import "testing"

func TestInterruptTypesAreClosed(t *testing.T) {
	for _, kind := range []InterruptType{InterruptApproval, InterruptQuestion} {
		if !kind.Valid() {
			t.Fatalf("declared kind %q is invalid", kind)
		}
	}
	for _, kind := range []InterruptType{"", "unknown"} {
		if kind.Valid() {
			t.Fatalf("undeclared kind %q is valid", kind)
		}
	}
}

func TestQuestionAnswerProjectionCannotChangeAcceptedResolution(t *testing.T) {
	original := [][]string{{"chosen"}}
	resolution, err := NewQuestionAnswer(original)
	if err != nil {
		t.Fatal(err)
	}
	original[0][0] = "edited input"
	projection := resolution.Question()
	projection.Answers()[0][0] = "edited projection"
	*projection = QuestionAnswer{}
	answers := resolution.Question().Answers()
	if len(answers) != 1 || len(answers[0]) != 1 || answers[0][0] != "chosen" {
		t.Fatalf("accepted resolution changed through its projection: %v", answers)
	}
}
