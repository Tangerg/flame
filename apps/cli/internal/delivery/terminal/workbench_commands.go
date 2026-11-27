package terminal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
)

func (c *promptComposer) moveDraftToHistory(message authoring.Message) error {
	sessionID := c.session.current.ID
	err := c.commitAuthoring(func() error { return c.workbench.MoveDraftToHistory(sessionID, message) })
	c.report(workbenchDraft, err)
	if err == nil {
		c.replaceDraft(authoring.Message{})
	}
	return err
}

func (c *promptComposer) stashPrompt() error {
	message, present, err := c.draft()
	if err != nil {
		return err
	}
	if !present {
		return errors.New("the composer has no prompt to stash")
	}
	sessionID := c.session.current.ID
	var stash workbench.Stash
	err = c.commitAuthoring(func() error {
		var err error
		stash, err = c.workbench.StashDraft(sessionID, message)
		return err
	})
	c.report(workbenchDraft, err)
	if err != nil {
		return fmt.Errorf("stash session draft: %w", err)
	}
	c.replaceDraft(authoring.Message{})
	c.notify("stashed prompt · " + stash.ID)
	return nil
}

func (c *promptComposer) showPromptStashes() {
	stashes := c.workbench.Stashes()
	if len(stashes) == 0 {
		c.notify("there are no prompt stashes")
		return
	}
	lines := make([]string, 0, len(stashes))
	for index, stash := range stashes {
		lines = append(lines, fmt.Sprintf("%d. %s · %s · %s", index+1, stash.ID, compactRelativeAge(stash.CreatedAt), promptSummary(stash)))
	}
	c.appendNote("prompt stashes", strings.Join(lines, "\n"))
}

func (c *promptComposer) applyPromptStash(identity string) error {
	if _, present, err := c.draft(); err != nil {
		return err
	} else if present {
		return errors.New("the composer is not empty; stash or clear it before applying another prompt")
	}
	stash, err := c.findPromptStash(identity)
	if err != nil {
		return err
	}
	if err := c.commitDraft(stash.Message); err != nil {
		return fmt.Errorf("apply prompt stash: save session draft: %w", err)
	}
	c.notify("applied prompt stash · " + stash.ID)
	return nil
}

func (c *promptComposer) deletePromptStash(identity string) error {
	stash, err := c.findPromptStash(identity)
	if err != nil {
		return err
	}
	deleted, err := c.workbench.DeleteStash(stash.ID)
	if err != nil {
		return err
	}
	if !deleted {
		return errors.New("prompt stash disappeared before it could be deleted")
	}
	c.notify("deleted prompt stash · " + stash.ID)
	return nil
}

func (c *promptComposer) findPromptStash(identity string) (workbench.Stash, error) {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return workbench.Stash{}, errors.New("a prompt stash id or unique prefix is required")
	}
	var matches []workbench.Stash
	for _, stash := range c.workbench.Stashes() {
		if strings.HasPrefix(stash.ID, identity) {
			matches = append(matches, stash)
		}
	}
	switch len(matches) {
	case 0:
		return workbench.Stash{}, fmt.Errorf("prompt stash %q was not found", identity)
	case 1:
		return matches[0], nil
	default:
		return workbench.Stash{}, fmt.Errorf("prompt stash prefix %q is ambiguous", identity)
	}
}

func promptSummary(stash workbench.Stash) string {
	text := strings.TrimSpace(stash.Message.Text)
	if line, _, ok := strings.Cut(text, "\n"); ok {
		text = strings.TrimSpace(line)
	}
	if text == "" && len(stash.Message.Attachments) > 0 {
		text = "@" + stash.Message.Attachments[0].Name
	}
	if text == "" {
		text = "empty prompt"
	}
	if len(stash.Message.Attachments) > 0 {
		text += format.Separator + countedNoun(len(stash.Message.Attachments), attachmentNoun)
	}
	return text
}
