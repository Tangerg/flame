package terminal

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// questionBlock separates an open human decision from its durable transcript
// fact. A pending Question occupies its stable transcript position without
// drawing an interrupt-looking prompt before the dialog owns input. Once the
// runtime accepts the answer, the same block becomes visible in place.
type questionBlock struct {
	theme    kit.Theme
	glyphs   kit.Glyphs
	question conversation.Question
	message  kit.Entry
}

var (
	_ headless.Block         = (*questionBlock)(nil)
	_ headless.TextProjector = (*questionBlock)(nil)
)

func newQuestionBlock(theme kit.Theme, glyphs kit.Glyphs, question conversation.Question) *questionBlock {
	block := &questionBlock{theme: theme, glyphs: glyphs}
	block.setQuestion(question)
	return block
}

func (q *questionBlock) answered() bool { return q.question.Answered() }

func (q *questionBlock) validateAccepted(question conversation.Question) error {
	if !question.Answered() {
		return fmt.Errorf("question %s has no accepted answers", question.ItemID)
	}
	expected, err := q.question.Accept(conversation.QuestionAnswer{Values: question.Answers})
	if err != nil {
		return err
	}
	if !expected.Equal(question) {
		return fmt.Errorf("accepted question %s differs from its pending transcript item", question.ItemID)
	}
	return nil
}

func (q *questionBlock) accept(question conversation.Question) {
	q.setQuestion(question)
}

func (q *questionBlock) HeightForWidth(width int) int {
	if !q.answered() {
		return 0
	}
	return q.message.HeightForWidth(width)
}

func (q *questionBlock) Draw(view grid.View) {
	if q.answered() {
		q.message.Draw(view)
	}
}

func (q *questionBlock) Rows(width int) []text.Row {
	if !q.answered() {
		return nil
	}
	return q.message.Rows(width)
}

func (q *questionBlock) setQuestion(question conversation.Question) {
	q.question = question.Clone()
	q.message = kit.Entry{
		Theme: q.theme, Label: question.Title,
		Body: presentQuestionBody(q.glyphs, question),
	}
}

func presentQuestionBody(glyphs kit.Glyphs, question conversation.Question) string {
	lines := make([]string, 0, len(question.Fields)*2)
	for index, field := range question.Fields {
		lines = append(lines, glyphs.Bullet+" "+field.Prompt)
		if question.Answered() {
			lines = append(lines, "  answer · "+strings.Join(question.Answers[index], ", "))
		}
	}
	body := strings.Join(lines, "\n")
	if question.Detail != "" {
		body = question.Detail + "\n" + body
	}
	return body
}
