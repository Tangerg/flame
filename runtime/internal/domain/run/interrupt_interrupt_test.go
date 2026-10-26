package run

import "testing"

func TestInterruptKindsAreClosed(t *testing.T) {
	for _, kind := range []InterruptKind{InterruptApproval, InterruptQuestion} {
		if !kind.Valid() {
			t.Fatalf("declared kind %q is invalid", kind)
		}
	}
	for _, kind := range []InterruptKind{"", "unknown"} {
		if kind.Valid() {
			t.Fatalf("undeclared kind %q is valid", kind)
		}
	}
}

func TestKeyIsStableAndKindSensitive(t *testing.T) {
	first := InterruptKey("approval", "shell", `{"command":"go test ./..."}`)
	if again := InterruptKey("approval", "shell", `{"command":"go test ./..."}`); again != first {
		t.Fatalf("Key() changed between equal inputs: %q != %q", first, again)
	}
	if question := InterruptKey("question", "shell", `{"command":"go test ./..."}`); question == first {
		t.Fatalf("Key() ignored interrupt kind: %q", first)
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
