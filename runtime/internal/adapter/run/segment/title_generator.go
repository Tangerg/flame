package segment

import (
	"context"
	"strings"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
)

const titleMaxInputRunes = 4000

const (
	titleModelInputBytes   = 20 * 1024
	titleModelOutputTokens = int64(64)
)

const titleMaxRunes = 80

type titleGenerator struct {
	model modeladapter.AuxiliaryResolver
}

const titlePrompt = `Write a concise title for a conversation that opens with the user message below.

Rules:
- 3 to 6 words, Title Case, at most 60 characters.
- Capture the task/topic; no filler ("Help with", "Question about").
- Output ONLY the title — no quotes, no surrounding punctuation, no markdown, no trailing period.`

func (g titleGenerator) Generate(ctx context.Context, firstMessage string) (string, error) {
	msg := strings.TrimSpace(firstMessage)
	if msg == "" {
		return "", nil
	}
	fallback := sanitizeTitle(msg)
	if runes := []rune(msg); len(runes) > titleMaxInputRunes {
		msg = string(runes[:titleMaxInputRunes])
	}
	text, err := g.model.Complete(ctx, modeladapter.AuxiliaryPrompt{
		Operation:    modeladapter.AuxiliaryTitle,
		SystemPrompt: titlePrompt, UserPrompt: msg,
		Budget: modeladapter.AuxiliaryBudget{MaxInputBytes: titleModelInputBytes, MaxOutputTokens: titleModelOutputTokens},
	})
	if err != nil {
		return fallback, err
	}
	title := sanitizeTitle(text)
	if title == "" {
		return fallback, nil
	}
	return title, nil
}

func sanitizeTitle(s string) string {
	s = strings.TrimSpace(s)
	if before, _, found := strings.Cut(s, "\n"); found {
		s = strings.TrimSpace(before)
	}
	s = strings.Trim(s, "\"'`")
	s = strings.TrimRight(s, ".。!?！？")
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > titleMaxRunes {
		s = strings.TrimSpace(string(r[:titleMaxRunes]))
	}
	return s
}
