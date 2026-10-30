package terminal

import (
	"slices"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestQuestionnaireOwnsAnswersAndNavigation(t *testing.T) {
	question := conversation.Question{
		RunID: "run_1", ItemID: "plan", Title: "Plan deployment",
		Fields: []conversation.QuestionField{
			{Prompt: "Goal", Kind: conversation.QuestionText},
			{Prompt: "Strategy", Kind: conversation.QuestionSingle, Options: []protocol.QuestionOption{{Label: "Safe"}, {Label: "Fast"}}},
			{Prompt: "Checks", Kind: conversation.QuestionMulti, Options: []protocol.QuestionOption{{Label: "Unit"}, {Label: "Integration"}}},
		},
	}
	previous := conversation.QuestionAnswer{Values: [][]string{{"release"}, {"Fast"}, {"Unit"}}}
	review, err := newQuestionnaire(question, previous)
	if err != nil {
		t.Fatal(err)
	}
	question.Fields[0].Prompt = "mutated by caller"
	previous.Values[0][0] = "mutated by caller"
	if review.Title() != "Plan deployment · 1/3" || review.response(0).text != "release" {
		t.Fatalf("initial questionnaire = %q, %q", review.Title(), review.response(0).text)
	}
	if !review.Advance() || review.Title() != "Plan deployment · 2/3" || !review.Advance() || review.Advance() {
		t.Fatalf("forward navigation stopped at %q", review.Title())
	}
	if !review.Back() || review.Title() != "Plan deployment · 2/3" {
		t.Fatalf("back navigation stopped at %q", review.Title())
	}

	answer, err := review.Answer()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"release"}, {"Fast"}, {"Unit"}}
	if !slices.EqualFunc(answer.Values, want, slices.Equal) {
		t.Fatalf("answer = %+v, want %+v", answer.Values, want)
	}
}

func TestQuestionnairePreservesOneLiteralCustomValue(t *testing.T) {
	question := conversation.Question{
		RunID: "run_1", ItemID: "targets", Title: "Targets",
		Fields: []conversation.QuestionField{{
			Prompt: "Targets", Kind: conversation.QuestionMulti, AllowCustom: true,
			Options: []protocol.QuestionOption{{Label: "linux"}, {Label: "darwin"}},
		}},
	}
	review, err := newQuestionnaire(question, nil)
	if err != nil {
		t.Fatal(err)
	}
	review.response(0).multiple = []questionChoice{offeredQuestionChoice("linux"), customQuestionChoice()}
	review.response(0).custom = "custom, darwin"
	if err := review.ConfirmCurrent(); err != nil {
		t.Fatal(err)
	}
	answer, err := review.Answer()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"linux", "custom, darwin"}
	if !slices.Equal(answer.Values[0], want) {
		t.Fatalf("custom values = %q, want %q", answer.Values[0], want)
	}
}

func TestQuestionnaireRestoresOfferedAndCustomChoices(t *testing.T) {
	question := conversation.Question{
		RunID: "run_1", ItemID: "targets", Title: "Targets",
		Fields: []conversation.QuestionField{
			{Prompt: "Platform", Kind: conversation.QuestionSingle, AllowCustom: true, Options: []protocol.QuestionOption{{Label: "linux"}, {Label: "darwin"}}},
			{Prompt: "Checks", Kind: conversation.QuestionMulti, AllowCustom: true, Options: []protocol.QuestionOption{{Label: "unit"}, {Label: "integration"}}},
		},
	}
	previous := conversation.QuestionAnswer{Values: [][]string{{"freebsd"}, {"unit", "smoke, manual"}}}
	review, err := newQuestionnaire(question, previous)
	if err != nil {
		t.Fatal(err)
	}
	if response := review.response(0); response == nil || response.single.kind != customChoice || response.custom != "freebsd" {
		t.Fatalf("restored single response = %+v", response)
	}
	wantChoices := []questionChoice{offeredQuestionChoice("unit"), customQuestionChoice()}
	if response := review.response(1); response == nil || !slices.Equal(response.multiple, wantChoices) || response.custom != "smoke, manual" {
		t.Fatalf("restored multiple response = %+v", response)
	}
	answer, err := review.Answer()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(answer.Values, previous.Values, slices.Equal) {
		t.Fatalf("restored answer = %+v, want %+v", answer.Values, previous.Values)
	}
}

func TestQuestionnaireRejectsRestoringMultipleCustomValues(t *testing.T) {
	question := conversation.Question{RunID: "run_1", ItemID: "q_1", Title: "Choices", Fields: []conversation.QuestionField{{Prompt: "Targets", Kind: conversation.QuestionMulti, AllowCustom: true, Options: []protocol.QuestionOption{{Label: "a"}, {Label: "b"}}}}}
	if _, err := newQuestionnaire(question, conversation.QuestionAnswer{Values: [][]string{{"custom one", "custom two"}}}); err == nil {
		t.Fatal("multiple custom values were restored as a valid field")
	}
}

func TestQuestionnaireRequiresExplicitConfirmationBeforeSkipping(t *testing.T) {
	question := conversation.Question{RunID: "run_1", ItemID: "q_1", Title: "Optional", Fields: []conversation.QuestionField{
		{Prompt: "Context", Kind: conversation.QuestionText},
		{Prompt: "Platform", Kind: conversation.QuestionSingle, Options: []protocol.QuestionOption{{Label: "a"}, {Label: "b"}}},
		{Prompt: "Checks", Kind: conversation.QuestionMulti, Options: []protocol.QuestionOption{{Label: "a"}, {Label: "b"}}},
	}}
	review, err := newQuestionnaire(question, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := review.Answer(); err == nil {
		t.Fatal("unconfirmed blank draft became a skip")
	}
	if err := review.ConfirmCurrent(); err != nil {
		t.Fatal(err)
	}
	review.Advance()
	review.response(1).single = skippedQuestionChoice()
	if err := review.ConfirmCurrent(); err != nil {
		t.Fatal(err)
	}
	review.Advance()
	if err := review.ConfirmCurrent(); err != nil {
		t.Fatal(err)
	}
	answer, err := review.Answer()
	if err != nil {
		t.Fatal(err)
	}
	if answer.Values == nil || len(answer.Values) != 3 {
		t.Fatalf("skip response = %#v", answer)
	}
	for _, values := range answer.Values {
		if values == nil || len(values) != 0 {
			t.Fatalf("explicit skipped field = %#v", values)
		}
	}
	restored, err := newQuestionnaire(question, answer)
	if err != nil {
		t.Fatal(err)
	}
	if restored.response(1).single.kind != skippedChoice {
		t.Fatal("restored skip selected an offered answer")
	}
}

func TestQuestionnaireRejectsMissingFieldsAndIncompleteAnswers(t *testing.T) {
	if _, err := newQuestionnaire(conversation.Question{}, nil); err == nil {
		t.Fatal("question without fields was accepted")
	}
	question := conversation.Question{
		RunID: "run_1", ItemID: "goal", Title: "Goal",
		Fields: []conversation.QuestionField{{Prompt: "Goal", Kind: conversation.QuestionText}},
	}
	review, err := newQuestionnaire(question, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := review.Answer(); err == nil {
		t.Fatal("unconfirmed field was accepted")
	}
}
