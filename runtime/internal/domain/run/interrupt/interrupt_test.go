package interrupt

import "testing"

func TestKindStringRoundTrip(t *testing.T) {
	for _, kind := range []Kind{Approval, Question} {
		parsed, ok := ParseKind(string(kind))
		if !ok || parsed != kind || !parsed.Valid() {
			t.Fatalf("ParseKind(%q) = %v, %t; want %v, true", string(kind), parsed, ok, kind)
		}
	}
	if parsed, ok := ParseKind("unknown"); ok || parsed.Valid() {
		t.Fatalf("ParseKind(unknown) = %v, %t; want invalid", parsed, ok)
	}
}

func TestKeyIsStableAndKindSensitive(t *testing.T) {
	first := Key("approval", "shell", `{"command":"go test ./..."}`)
	if again := Key("approval", "shell", `{"command":"go test ./..."}`); again != first {
		t.Fatalf("Key() changed between equal inputs: %q != %q", first, again)
	}
	if question := Key("question", "shell", `{"command":"go test ./..."}`); question == first {
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
