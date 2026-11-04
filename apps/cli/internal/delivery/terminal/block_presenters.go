package terminal

import (
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/kit"
)

func presentUser(p blockPresentation, block conversation.Block) []base.Block {
	body := strings.TrimSpace(block.Text)
	if len(block.Attachments) > 0 {
		lines := make([]string, 0, len(block.Attachments))
		for _, item := range block.Attachments {
			lines = append(lines, "@"+item.Name+" · "+item.MimeType)
		}
		if body != "" {
			body += "\n\n"
		}
		body += strings.Join(lines, "\n")
	}
	speaker := p.Speaker
	if speaker == "" {
		speaker = selfSpeaker
	}
	return []base.Block{newUserMessageBlock(p.Theme, speaker, body)}
}

func presentMarkdown(speaker string) func(blockPresentation, conversation.Block) []base.Block {
	return func(p blockPresentation, block conversation.Block) []base.Block {
		label := speaker
		if p.Speaker != "" {
			label = p.Speaker
		}
		message := &markdownBlock{theme: p.Theme, speaker: label}
		look := p.Look
		if block.Kind == conversation.BlockReasoning {
			look.Text, look.Strong = p.Theme.Muted, p.Theme.Subtle
		}
		message.setSource(block.Text, look)
		rendered := []base.Block{message}
		for _, image := range block.Images {
			if p.Image == nil {
				rendered = append(rendered, fallbackInlineImage(p.Theme, image))
				continue
			}
			rendered = append(rendered, p.Image(image))
		}
		return rendered
	}
}

func presentTool(p blockPresentation, block conversation.Block) []base.Block {
	return []base.Block{newToolBlock(p, block)}
}

func presentQuestion(p blockPresentation, block conversation.Block) []base.Block {
	if block.Question == nil {
		return nil
	}
	return []base.Block{newQuestionBlock(p.Theme, p.Glyphs, *block.Question)}
}

func presentNotice(p blockPresentation, block conversation.Block) []base.Block {
	return []base.Block{&kit.Entry{Theme: p.Theme, Label: "notice", Body: block.Text}}
}

func presentFailure(p blockPresentation, block conversation.Block) []base.Block {
	return []base.Block{presentError(p.Theme, block.Text)}
}
