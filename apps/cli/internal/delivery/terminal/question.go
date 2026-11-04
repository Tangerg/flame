package terminal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
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

func (p *interruptPrompts) openQuestion(question conversation.Question) {
	review, err := newQuestionnaire(question, p.review.CurrentAnswer())
	if err != nil {
		p.run.fail(err)
		return
	}
	p.questionnaire = review
	p.openQuestionField()
}

func (p *interruptPrompts) openQuestionField() {
	review := p.questionnaire
	index, _, ok := review.Current()
	if !ok {
		p.run.fail(errors.New("questionnaire has no current field"))
		return
	}
	response := review.response(index)
	if response == nil {
		p.run.fail(errors.New("questionnaire has no response for the current field"))
		return
	}
	fields, err := p.buildQuestionFields(response)
	if err != nil {
		p.run.fail(err)
		return
	}
	response.confirmed = false
	p.showQuestionDialog(review, fields)
}

func (p *interruptPrompts) buildQuestionFields(response *questionResponse) ([]base.Field, error) {
	specification := response.field
	label := questionFieldLabel(specification)
	switch specification.Kind {
	case conversation.QuestionText:
		return []base.Field{p.buildQuestionText(&response.text, label, "Leave empty to skip this field", func(value string) error {
			return response.contract.ValidateField(response.index, textQuestionValues(value))
		})}, nil
	case conversation.QuestionSingle:
		fields := []base.Field{p.buildQuestionSingle(response, specification, label)}
		return p.appendCustomQuestionField(fields, response), nil
	case conversation.QuestionMulti:
		fields := []base.Field{p.buildQuestionMulti(response, specification, label)}
		return p.appendCustomQuestionField(fields, response), nil
	default:
		return nil, errors.New("runtime returned an unsupported question field kind")
	}
}

func (p *interruptPrompts) buildQuestionText(value *string, label, placeholder string, check func(string) error) base.Field {
	field := base.NewText(base.TextConfig{Label: label, Placeholder: placeholder, Initial: *value, Check: check})
	field.Clipboard = p.clipboard
	return field
}

func (p *interruptPrompts) buildQuestionSingle(response *questionResponse, specification conversation.QuestionField, label string) base.Field {
	options := append(questionOptions(specification), base.Option[questionChoice]{Label: "Skip this field", Value: skippedQuestionChoice()})
	field := base.NewSelect(base.SelectConfig[questionChoice]{
		Label: label, Initial: response.single, Options: options, Rows: min(len(options), 5),
		Same: sameQuestionChoice,
	})
	return field
}

func (p *interruptPrompts) buildQuestionMulti(response *questionResponse, specification conversation.QuestionField, label string) base.Field {
	options := questionOptions(specification)
	field := base.NewMultiSelect(base.MultiSelectConfig[questionChoice]{
		Label: label, Initial: response.multiple, Options: options, Rows: min(len(options), 5),
		Same: sameQuestionChoice,
	})
	return field
}

func (p *interruptPrompts) appendCustomQuestionField(fields []base.Field, response *questionResponse) []base.Field {
	if !response.field.AllowCustom {
		return fields
	}
	placeholder := "One value when Other is selected; commas are kept"
	return append(fields, p.buildQuestionText(&response.custom, "Custom answer", placeholder, nil))
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

func (p *interruptPrompts) showQuestionDialog(review *questionnaire, fields []base.Field) {
	form := kit.FormConfig{Fields: fields, Theme: p.theme, Glyphs: p.glyphs,
		Title: strings.Join(nonEmptyStrings([]string{
			p.review.SubmissionFailure(), review.question.Detail,
		}), " · "),
		Hints: []keymap.Action{base.Submit, base.Cancel}}
	response := review.response(review.current)
	collect := func() questionResponse {
		candidate := *response
		switch field := fields[0].(type) {
		case *base.Text:
			candidate.text = field.Text()
		case *base.Select[questionChoice]:
			if chosen, ok := field.Chosen(); ok {
				candidate.single = chosen.Value
			}
		case *base.MultiSelect[questionChoice]:
			candidate.multiple = field.Taken()
		}
		if candidate.field.AllowCustom && len(fields) > 1 {
			candidate.custom = fields[len(fields)-1].(*base.Text).Text()
		}
		return candidate
	}
	form.Check = func() error { candidate := collect(); _, err := candidate.values(); return err }
	var dialog *presentationDialog
	form.Submit = func(base.SubmissionID) (scene.ActionState, error) {
		if p.questionnaire == review && p.questionDialog == dialog {
			*response = collect()
			p.advanceQuestionnaire()
		}
		return scene.Applied, nil
	}
	leave := func() {
		if p.questionnaire == review && p.questionDialog == dialog {
			*response = collect()
			p.backOrCancelQuestionnaire()
		}
	}
	dressed := kit.NewForm(form)
	title := review.Title()
	if review.question.Fields[review.current].Kind != conversation.QuestionSingle {
		title += " · empty skips"
	}
	dialog = newPresentationDialog(p.stack, kit.DialogConfig{Theme: p.theme, Glyphs: p.glyphs,
		Title: title, Body: formNavigation(dressed.Element(), leave),
		Where: layout.Placement{Width: 88, Height: formDialogHeight(dressed.HeightForWidth(80), len(fields), 18)},
	})
	p.questionDialog = dialog
	dialog.Show()
}

func (p *interruptPrompts) advanceQuestionnaire() {
	review := p.questionnaire
	if review == nil {
		return
	}
	if err := review.ConfirmCurrent(); err != nil {
		p.run.fail(err)
		return
	}
	p.questionDialog.Dismiss()
	p.questionDialog = nil
	if review.Advance() {
		p.openQuestionField()
		return
	}
	p.finishQuestionnaire(false)
}

func (p *interruptPrompts) backOrCancelQuestionnaire() {
	review := p.questionnaire
	if review == nil {
		return
	}
	p.questionDialog.Dismiss()
	p.questionDialog = nil
	if review.Back() {
		p.openQuestionField()
		return
	}
	p.finishQuestionnaire(true)
}

func (p *interruptPrompts) finishQuestionnaire(canceled bool) {
	review := p.questionnaire
	if review == nil {
		return
	}
	if p.questionDialog != nil {
		p.questionDialog.Dismiss()
		p.questionDialog = nil
	}
	if canceled {
		p.questionnaire = nil
		if p.backInterrupt() {
			return
		}
		p.abort("question canceled by the terminal user")
		return
	}
	answer, err := review.Answer()
	if err != nil {
		p.run.fail(err)
		return
	}
	if err := p.review.Record(answer); err != nil {
		p.run.fail(fmt.Errorf("record question: %w", err))
		return
	}
	p.questionnaire = nil
	p.advanceInterruptReview()
}

func questionOptions(field conversation.QuestionField) []base.Option[questionChoice] {
	out := make([]base.Option[questionChoice], 0, len(field.Options)+1)
	for _, option := range field.Options {
		label := option.Label
		if option.Description != "" {
			label += " — " + option.Description
		}
		if option.Preview != "" {
			label += " · " + option.Preview
		}
		out = append(out, base.Option[questionChoice]{Label: label, Value: offeredQuestionChoice(option.Label)})
	}
	if field.AllowCustom {
		out = append(out, base.Option[questionChoice]{Label: "Other — provide a custom answer", Value: customQuestionChoice()})
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
