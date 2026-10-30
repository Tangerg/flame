package terminal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

type questionnaire struct {
	question  conversation.Question
	contract  questioncontract.Contract
	current   int
	responses []questionResponse
}

type questionResponse struct {
	field     conversation.QuestionField
	contract  questioncontract.Contract
	index     int
	confirmed bool
	text      string
	single    questionChoice
	multiple  []questionChoice
	custom    string
}

type questionChoiceKind uint8

const (
	offeredChoice questionChoiceKind = iota + 1
	customChoice
	skippedChoice
)

type questionChoice struct {
	kind  questionChoiceKind
	value string
}

func offeredQuestionChoice(value string) questionChoice {
	return questionChoice{kind: offeredChoice, value: value}
}
func customQuestionChoice() questionChoice  { return questionChoice{kind: customChoice} }
func skippedQuestionChoice() questionChoice { return questionChoice{kind: skippedChoice} }

func newQuestionnaire(question conversation.Question, previous conversation.Answer) (*questionnaire, error) {
	owned := question.Clone()
	contract, err := owned.AnswerContract()
	if err != nil {
		return nil, err
	}
	review := &questionnaire{question: owned, contract: contract, responses: make([]questionResponse, len(owned.Fields))}
	answer, hasPrevious := previous.(conversation.QuestionAnswer)
	if previous != nil && !hasPrevious {
		return nil, errors.New("questionnaire received another answer kind")
	}
	if hasPrevious {
		if err := contract.Validate(answer.Values); err != nil {
			return nil, err
		}
	}
	for index, field := range owned.Fields {
		response := questionResponse{field: field, contract: contract, index: index}
		if field.Kind == conversation.QuestionSingle && len(field.Options) > 0 {
			response.single = offeredQuestionChoice(field.Options[0].Label)
		}
		if hasPrevious {
			response.restore(answer.Values[index])
			response.confirmed = true
		}
		review.responses[index] = response
	}
	return review, nil
}

func (q *questionResponse) restore(values []string) {
	switch q.field.Kind {
	case conversation.QuestionText:
		if len(values) > 0 {
			q.text = values[0]
		}
	case conversation.QuestionSingle:
		if len(values) == 0 {
			q.single = skippedQuestionChoice()
			return
		}
		if fieldOffers(q.field, values[0]) || !q.field.AllowCustom {
			q.single = offeredQuestionChoice(values[0])
			return
		}
		q.single = customQuestionChoice()
		q.custom = values[0]
	case conversation.QuestionMulti:
		hasCustom := false
		for _, value := range values {
			if fieldOffers(q.field, value) || !q.field.AllowCustom {
				q.multiple = append(q.multiple, offeredQuestionChoice(value))
				continue
			}
			q.custom = value
			hasCustom = true
		}
		if hasCustom {
			q.multiple = append(q.multiple, customQuestionChoice())
		}
	}
}

func (q *questionnaire) Current() (int, conversation.QuestionField, bool) {
	if q.current < 0 || q.current >= len(q.question.Fields) {
		return 0, conversation.QuestionField{}, false
	}
	return q.current, q.question.Fields[q.current], true
}

func (q *questionnaire) Advance() bool {
	if q.current+1 >= len(q.question.Fields) {
		return false
	}
	q.current++
	return true
}

func (q *questionnaire) Back() bool {
	if q.current == 0 {
		return false
	}
	q.current--
	return true
}

func (q *questionnaire) Title() string {
	if len(q.question.Fields) <= 1 {
		return q.question.Title
	}
	return fmt.Sprintf("%s · %d/%d", q.question.Title, q.current+1, len(q.question.Fields))
}

func (q *questionnaire) response(index int) *questionResponse {
	if index < 0 || index >= len(q.responses) {
		return nil
	}
	return &q.responses[index]
}

func (q *questionnaire) Answer() (conversation.QuestionAnswer, error) {
	if q == nil {
		return conversation.QuestionAnswer{}, errors.New("questionnaire is not active")
	}
	answer := conversation.QuestionAnswer{Values: make([][]string, len(q.question.Fields))}
	for index := range q.responses {
		if !q.responses[index].confirmed {
			return conversation.QuestionAnswer{}, fmt.Errorf("question field %d has not been confirmed", index+1)
		}
		values, err := q.responses[index].values()
		if err != nil {
			return conversation.QuestionAnswer{}, fmt.Errorf("answer question field %d: %w", index+1, err)
		}
		answer.Values[index] = values
	}
	if err := q.contract.Validate(answer.Values); err != nil {
		return conversation.QuestionAnswer{}, fmt.Errorf("answer question: %w", err)
	}
	return answer, nil
}

func (q *questionnaire) ConfirmCurrent() error {
	response := q.response(q.current)
	if response == nil {
		return errors.New("questionnaire has no current response")
	}
	if _, err := response.values(); err != nil {
		return err
	}
	response.confirmed = true
	return nil
}

func textQuestionValues(value string) []string {
	if value == "" {
		return []string{}
	}
	return []string{value}
}

func (q *questionResponse) values() ([]string, error) {
	var values []string
	switch q.field.Kind {
	case conversation.QuestionText:
		values = textQuestionValues(q.text)
	case conversation.QuestionSingle:
		switch q.single.kind {
		case offeredChoice:
			values = []string{q.single.value}
		case customChoice:
			values = []string{q.custom}
		case skippedChoice:
			values = []string{}
		default:
			return nil, errors.New("question choice is not selected")
		}
	case conversation.QuestionMulti:
		var err error
		values, err = q.multipleValues(q.multiple, q.custom)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("runtime returned an unsupported question field kind")
	}
	if err := q.contract.ValidateField(q.index, values); err != nil {
		return nil, err
	}
	return values, nil
}

func (q *questionResponse) multipleValues(choices []questionChoice, custom string) ([]string, error) {
	values := make([]string, 0, len(choices))
	for _, choice := range choices {
		switch choice.kind {
		case offeredChoice:
			values = append(values, choice.value)
		case customChoice:
			values = append(values, custom)
		default:
			return nil, errors.New("multiple-choice editor contains an unsupported selection")
		}
	}
	return values, nil
}

func (a *app) openQuestion(question conversation.Question) {
	review, err := newQuestionnaire(question, a.dialogs.interruptReview.CurrentAnswer())
	if err != nil {
		a.fail(err)
		return
	}
	a.dialogs.questionnaire = review
	a.openQuestionField()
}

func (a *app) openQuestionField() {
	review := a.dialogs.questionnaire
	index, _, ok := review.Current()
	if !ok {
		a.fail(errors.New("questionnaire has no current field"))
		return
	}
	response := review.response(index)
	if response == nil {
		a.fail(errors.New("questionnaire has no response for the current field"))
		return
	}
	fields, err := a.buildQuestionFields(response)
	if err != nil {
		a.fail(err)
		return
	}
	response.confirmed = false
	a.showQuestionDialog(review, fields)
}

func (a *app) buildQuestionFields(response *questionResponse) ([]headless.Field, error) {
	specification := response.field
	label := questionFieldLabel(specification)
	switch specification.Kind {
	case conversation.QuestionText:
		return []headless.Field{a.buildQuestionText(&response.text, label, "Leave empty to skip this field", func(value string) error {
			return response.contract.ValidateField(response.index, textQuestionValues(value))
		})}, nil
	case conversation.QuestionSingle:
		fields := []headless.Field{a.buildQuestionSingle(response, specification, label)}
		return a.appendCustomQuestionField(fields, response), nil
	case conversation.QuestionMulti:
		fields := []headless.Field{a.buildQuestionMulti(response, specification, label)}
		return a.appendCustomQuestionField(fields, response), nil
	default:
		return nil, errors.New("runtime returned an unsupported question field kind")
	}
}

func (a *app) buildQuestionText(value *string, label, placeholder string, check func(string) error) headless.Field {
	field := headless.NewText(headless.TextConfig{Label: label, Placeholder: placeholder, Value: headless.Bind(value), Check: check})
	field.Clipboard = a.loop.Clipboard()
	return field
}

func (a *app) buildQuestionSingle(response *questionResponse, specification conversation.QuestionField, label string) headless.Field {
	options := append(questionOptions(specification), headless.Option[questionChoice]{Label: "Skip this field", Value: skippedQuestionChoice()})
	field := headless.NewSelect(headless.SelectConfig[questionChoice]{
		Label: label, Value: headless.Bind(&response.single), Rows: min(len(options), 5),
		Same: sameQuestionChoice,
	})
	field.SetOptions(options)
	return field
}

func (a *app) buildQuestionMulti(response *questionResponse, specification conversation.QuestionField, label string) headless.Field {
	options := questionOptions(specification)
	field := headless.NewMultiSelect(headless.MultiSelectConfig[questionChoice]{
		Label: label, Value: headless.Bind(&response.multiple), Rows: min(len(options), 5),
		Same: sameQuestionChoice, Check: func(values []questionChoice) error {
			candidate, err := response.multipleValues(values, response.custom)
			if err != nil {
				return err
			}
			return response.contract.ValidateField(response.index, candidate)
		},
	})
	field.SetOptions(options)
	return field
}

func (a *app) appendCustomQuestionField(fields []headless.Field, response *questionResponse) []headless.Field {
	if !response.field.AllowCustom {
		return fields
	}
	placeholder := "One value when Other is selected; commas are kept"
	check := func(value string) error {
		if !response.choosesCustom() {
			return nil
		}
		_, err := response.values()
		return err
	}
	return append(fields, a.buildQuestionText(&response.custom, "Custom answer", placeholder, check))
}

func questionFieldLabel(specification conversation.QuestionField) string {
	if specification.Header == "" {
		return specification.Prompt
	}
	return specification.Header + " — " + specification.Prompt
}

func (q *questionResponse) choosesCustom() bool {
	if q.field.Kind == conversation.QuestionSingle {
		return q.single.kind == customChoice
	}
	for _, choice := range q.multiple {
		if choice.kind == customChoice {
			return true
		}
	}
	return false
}

func (a *app) showQuestionDialog(review *questionnaire, fields []headless.Field) {
	form := headless.NewForm(fields...)
	var dialog *kit.Dialog
	form.Done = func() {
		if a.dialogs.questionnaire == review && a.dialogs.questionDialog == dialog {
			a.advanceQuestionnaire()
		}
	}
	form.GaveUp = func() {
		if a.dialogs.questionnaire == review && a.dialogs.questionDialog == dialog {
			a.backOrCancelQuestionnaire()
		}
	}
	dressed := kit.NewForm(kit.FormConfig{
		Theme: a.transcript.theme, Glyphs: a.transcript.glyphs, Controller: form,
		Title: strings.Join(nonEmptyStrings([]string{
			a.dialogs.interruptReview.SubmissionFailure(), review.question.Detail,
		}), " · "),
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})
	title := review.Title()
	if review.question.Fields[review.current].Kind != conversation.QuestionSingle {
		title += " · empty skips"
	}
	dialog = kit.NewDialog(kit.DialogConfig{
		Stack: &a.stack, Theme: a.transcript.theme, Glyphs: a.transcript.glyphs,
		Title: title, Body: dressed,
		Where: layout.Placement{Width: 88, Height: formDialogHeight(dressed.HeightForWidth(80), len(fields), 18)},
	})
	a.dialogs.questionDialog = dialog
	dialog.Controller().Show()
}

func (a *app) advanceQuestionnaire() {
	review := a.dialogs.questionnaire
	if review == nil {
		return
	}
	if err := review.ConfirmCurrent(); err != nil {
		a.fail(err)
		return
	}
	a.dialogs.questionDialog.Controller().Dismiss()
	a.dialogs.questionDialog = nil
	if review.Advance() {
		a.openQuestionField()
		return
	}
	a.finishQuestionnaire(false)
}

func (a *app) backOrCancelQuestionnaire() {
	review := a.dialogs.questionnaire
	if review == nil {
		return
	}
	a.dialogs.questionDialog.Controller().Dismiss()
	a.dialogs.questionDialog = nil
	if review.Back() {
		a.openQuestionField()
		return
	}
	a.finishQuestionnaire(true)
}

func (a *app) finishQuestionnaire(canceled bool) {
	review := a.dialogs.questionnaire
	if review == nil {
		return
	}
	if a.dialogs.questionDialog != nil {
		a.dialogs.questionDialog.Controller().Dismiss()
		a.dialogs.questionDialog = nil
	}
	if canceled {
		a.dialogs.questionnaire = nil
		if a.backInterrupt() {
			return
		}
		a.abortInterrupts("question canceled by the terminal user")
		return
	}
	answer, err := review.Answer()
	if err != nil {
		a.fail(err)
		return
	}
	if err := a.dialogs.interruptReview.Record(answer); err != nil {
		a.fail(fmt.Errorf("record question: %w", err))
		return
	}
	a.dialogs.questionnaire = nil
	a.advanceInterruptReview()
}

func questionOptions(field conversation.QuestionField) []headless.Option[questionChoice] {
	out := make([]headless.Option[questionChoice], 0, len(field.Options)+1)
	for _, option := range field.Options {
		label := option.Label
		if option.Description != "" {
			label += " — " + option.Description
		}
		if option.Preview != "" {
			label += " · " + option.Preview
		}
		out = append(out, headless.Option[questionChoice]{Label: label, Value: offeredQuestionChoice(option.Label)})
	}
	if field.AllowCustom {
		out = append(out, headless.Option[questionChoice]{Label: "Other — provide a custom answer", Value: customQuestionChoice()})
	}
	return out
}

func fieldOffers(field conversation.QuestionField, value string) bool {
	for _, option := range field.Options {
		if option.Label == value {
			return true
		}
	}
	return false
}

func sameQuestionChoice(left, right questionChoice) bool {
	return left == right
}

func requiredText(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("an answer is required")
	}
	return nil
}
