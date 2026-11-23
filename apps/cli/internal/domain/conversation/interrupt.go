package conversation

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
)

var ErrUnsupportedInterrupt = errors.New("interrupt is unsupported")

func InterruptItemID(interrupt Interrupt) string {
	switch item := interrupt.(type) {
	case Approval:
		return item.ItemID
	case Question:
		return item.ItemID
	default:
		return ""
	}
}

func InterruptRunID(interrupt Interrupt) string {
	switch item := interrupt.(type) {
	case Approval:
		return item.RunID
	case Question:
		return item.RunID
	default:
		return ""
	}
}

func validateInterrupt(interrupt Interrupt) error {
	switch item := interrupt.(type) {
	case Approval:
		return item.Validate()
	case Question:
		if item.Answered() {
			return errors.New("interrupt question already has accepted answers")
		}
		return item.Validate()
	case nil:
		return errors.New("interrupt is nil")
	default:
		return fmt.Errorf("%w: %T", ErrUnsupportedInterrupt, interrupt)
	}
}

func ValidateInterrupts(interrupts []Interrupt) error {
	if len(interrupts) == 0 {
		return errors.New("interrupts are empty")
	}
	seen := make(map[string]struct{}, len(interrupts))
	for i, interrupt := range interrupts {
		if err := validateInterrupt(interrupt); err != nil {
			return fmt.Errorf("interrupt %d: %w", i+1, err)
		}
		id := InterruptItemID(interrupt)
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("interrupt item id %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (a Approval) Validate() error {
	var problems []error
	if err := protocol.ValidateRunID(a.RunID); err != nil {
		problems = append(problems, err)
	}
	if err := protocol.ValidateItemID(a.ItemID); err != nil {
		problems = append(problems, err)
	}
	if strings.TrimSpace(a.Title) == "" {
		problems = append(problems, errors.New("title is empty"))
	}
	if a.Tool == nil {
		problems = append(problems, errors.New("tool is absent"))
	} else {
		if err := a.Tool.Validate(); err != nil {
			problems = append(problems, err)
		}
		if a.Tool.Status != ToolRunning {
			problems = append(problems, errors.New("tool is not running"))
		}
	}
	if a.Risk != "" && !slices.Contains([]protocol.ApprovalRisk{protocol.ApprovalRiskLow, protocol.ApprovalRiskMedium, protocol.ApprovalRiskHigh}, a.Risk) {
		problems = append(problems, fmt.Errorf("risk %q is invalid", a.Risk))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("approval: %w", err)
	}
	return nil
}

func (a Approval) Clone() Approval {
	if a.Tool != nil {
		tool := a.Tool.Clone()
		a.Tool = &tool
	}
	return a
}

func (a Approval) Equal(other Approval) bool {
	if a.RunID != other.RunID || a.ItemID != other.ItemID || a.Title != other.Title || a.Detail != other.Detail ||
		a.Risk != other.Risk ||
		a.Rememberable != other.Rememberable || (a.Tool == nil) != (other.Tool == nil) {
		return false
	}
	return a.Tool == nil || a.Tool.Equal(*other.Tool)
}

func (q Question) Validate() error {
	var problems []error
	if err := protocol.ValidateRunID(q.RunID); err != nil {
		problems = append(problems, err)
	}
	if err := protocol.ValidateItemID(q.ItemID); err != nil {
		problems = append(problems, err)
	}
	if strings.TrimSpace(q.Title) == "" {
		problems = append(problems, errors.New("title is empty"))
	}
	wire := protocol.Question{Fields: make([]protocol.QuestionField, len(q.Fields)), Answers: q.Answers}
	for i, field := range q.Fields {
		translated, err := field.wire()
		if err != nil {
			problems = append(problems, fmt.Errorf("field %d: %w", i+1, err))
			continue
		}
		wire.Fields[i] = translated
	}
	if err := protocol.ValidateWireTree(wire); err != nil {
		problems = append(problems, err)
	}
	contract, contractErr := q.AnswerContract()
	if contractErr != nil {
		problems = append(problems, contractErr)
	} else if q.Answered() {
		if err := contract.Validate(q.Answers); err != nil {
			problems = append(problems, fmt.Errorf("accepted answer: %w", err))
		}
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("question: %w", err)
	}
	return nil
}

func (q Question) Answered() bool { return q.Answers != nil }

func (q Question) Clone() Question { return cloneQuestion(q) }

func (q Question) Equal(other Question) bool {
	return q.RunID == other.RunID && q.ItemID == other.ItemID && q.Title == other.Title &&
		q.Answered() == other.Answered() &&
		slices.EqualFunc(q.Fields, other.Fields, func(left, right QuestionField) bool {
			return left.Equal(right)
		}) && equalAnswerValues(q.Answers, other.Answers)
}

func (q QuestionField) Validate() error {
	wire, err := q.wire()
	if err != nil {
		return err
	}
	if err := protocol.ValidateWireTree(wire); err != nil {
		return err
	}
	_, err = (Question{Fields: []QuestionField{q}}).AnswerContract()
	return err
}

func (q QuestionField) wire() (protocol.QuestionField, error) {
	wire := protocol.QuestionField{
		Prompt: q.Prompt, Header: q.Header, AllowCustom: q.AllowCustom,
		Options: q.Options,
	}
	switch q.Kind {
	case QuestionText:
		wire.Type = protocol.QuestionFieldText
	case QuestionSingle, QuestionMulti:
		wire.Type = protocol.QuestionFieldChoice
		wire.Multiple = q.Kind == QuestionMulti
	default:
		return protocol.QuestionField{}, fmt.Errorf("kind %q is invalid", q.Kind)
	}
	return wire, nil
}

func (q QuestionField) Equal(other QuestionField) bool {
	return q.Prompt == other.Prompt && q.Header == other.Header && q.Kind == other.Kind &&
		q.AllowCustom == other.AllowCustom && slices.Equal(q.Options, other.Options)
}

func ValidateAnswer(interrupt Interrupt, answer Answer) error {
	if err := validateInterrupt(interrupt); err != nil {
		return err
	}
	switch item := interrupt.(type) {
	case Approval:
		provided, ok := answer.(ApprovalAnswer)
		if !ok {
			return errors.New("approval requires an approval answer")
		}
		if !item.Rememberable && provided.Remember != "" {
			return errors.New("approval cannot be remembered")
		}
		return provided.Validate()
	case Question:
		provided, ok := answer.(QuestionAnswer)
		if !ok {
			return errors.New("question requires a question answer")
		}
		return validateQuestionAnswer(item, provided)
	default:
		return fmt.Errorf("%w: %T", ErrUnsupportedInterrupt, interrupt)
	}
}

func (a ApprovalAnswer) Validate() error {
	if err := protocol.ValidateWireTree((protocol.InterruptResponseValue{
		Type: protocol.InterruptResponseApproval, Decision: a.Decision,
	})); err != nil {
		return fmt.Errorf("approval answer: %w", err)
	}
	if a.Remember != "" {
		if err := protocol.ValidateWireTree((protocol.RememberScope{Scope: a.Remember})); err != nil {
			return fmt.Errorf("approval answer: %w", err)
		}
	}
	if a.ArgumentOverride != nil {
		if a.Decision != protocol.ApprovalApprove {
			return errors.New("approval answer: denied tool carries an argument override")
		}
		if err := a.ArgumentOverride.Validate(); err != nil {
			return fmt.Errorf("approval answer: %w", err)
		}
	}
	return nil
}

func (q Question) AnswerContract() (questioncontract.Contract, error) {
	if err := questioncontract.ValidateFieldCount(len(q.Fields)); err != nil {
		return questioncontract.Contract{}, err
	}
	fields := make([]questioncontract.Field, len(q.Fields))
	for index, field := range q.Fields {
		wire, err := field.wire()
		if err != nil {
			return questioncontract.Contract{}, err
		}
		labels := make([]string, len(wire.Options))
		for i, option := range wire.Options {
			labels[i] = option.Label
		}
		fields[index] = questioncontract.Field{Kind: questioncontract.Kind(wire.Type), Options: labels, Multiple: wire.Multiple, AllowCustom: wire.AllowCustom}
	}
	return questioncontract.New(fields)
}

func validateQuestionAnswer(question Question, answer QuestionAnswer) error {
	contract, err := question.AnswerContract()
	if err != nil {
		return err
	}
	return contract.Validate(answer.Values)
}

func CloneInterrupt(interrupt Interrupt) Interrupt {
	switch item := interrupt.(type) {
	case Approval:
		return item.Clone()
	case Question:
		return item.Clone()
	default:
		return nil
	}
}

func cloneQuestion(question Question) Question {
	question.Fields = slices.Clone(question.Fields)
	for i := range question.Fields {
		question.Fields[i].Options = slices.Clone(question.Fields[i].Options)
	}
	question.Answers = CloneAnswers(question.Answers)
	return question
}

func CloneAnswers(values [][]string) [][]string {
	if values == nil {
		return nil
	}
	cloned := make([][]string, len(values))
	for i, answers := range values {
		cloned[i] = slices.Clone(answers)
	}
	return cloned
}

func equalAnswerValues(left, right [][]string) bool {
	return slices.EqualFunc(left, right, func(left, right []string) bool {
		return slices.Equal(left, right)
	})
}

func CloneInterrupts(interrupts []Interrupt) []Interrupt {
	out := make([]Interrupt, len(interrupts))
	for i, interrupt := range interrupts {
		out[i] = CloneInterrupt(interrupt)
	}
	return out
}

func CloneAnswer(answer Answer) Answer {
	switch item := answer.(type) {
	case ApprovalAnswer:
		item.ArgumentOverride = item.ArgumentOverride.Clone()
		return item
	case QuestionAnswer:
		cloned := item
		cloned.Values = CloneAnswers(item.Values)
		return cloned
	default:
		return nil
	}
}

func answerEqual(left, right Answer) bool {
	switch typed := left.(type) {
	case ApprovalAnswer:
		other, ok := right.(ApprovalAnswer)
		return ok && typed.Decision == other.Decision && typed.Remember == other.Remember &&
			typed.Reason == other.Reason && typed.ArgumentOverride.Equal(other.ArgumentOverride)
	case QuestionAnswer:
		other, ok := right.(QuestionAnswer)
		return ok && equalAnswerValues(typed.Values, other.Values)
	default:
		return left == nil && right == nil
	}
}

func InterruptsEqual(left, right []Interrupt) bool {
	return slices.EqualFunc(left, right, func(left, right Interrupt) bool {
		switch typed := left.(type) {
		case Approval:
			other, ok := right.(Approval)
			return ok && typed.Equal(other)
		case Question:
			other, ok := right.(Question)
			return ok && typed.Equal(other)
		default:
			return left == nil && right == nil
		}
	})
}
