// Package prompt owns CLI-authored messages, options and immutable command input.
package prompt

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

const (
	MaxMessageAttachments = 16
	MaxMessageTextBytes   = 4 << 20
)

type Message struct {
	Text        string
	Attachments []Attachment
}

func (m Message) Validate() error {
	if len(m.Text) > MaxMessageTextBytes {
		return fmt.Errorf("message text has %d bytes; limit is %d", len(m.Text), MaxMessageTextBytes)
	}
	if m.IsEmpty() {
		return errors.New("message is empty")
	}
	if len(m.Attachments) > MaxMessageAttachments {
		return fmt.Errorf("message has %d attachments; limit is %d", len(m.Attachments), MaxMessageAttachments)
	}
	ids := make(map[string]struct{}, len(m.Attachments))
	paths := make(map[string]struct{}, len(m.Attachments))
	for i, attachment := range m.Attachments {
		if err := attachment.Validate(); err != nil {
			return fmt.Errorf("message attachment %d: %w", i+1, err)
		}
		if strings.TrimSpace(attachment.Path) == "" {
			return fmt.Errorf("message attachment %d: local path is empty", i+1)
		}
		if _, duplicate := ids[attachment.ID]; duplicate {
			return fmt.Errorf("message repeats attachment id %q", attachment.ID)
		}
		if _, duplicate := paths[attachment.Path]; duplicate {
			return fmt.Errorf("message repeats attachment path %q", attachment.Path)
		}
		ids[attachment.ID] = struct{}{}
		paths[attachment.Path] = struct{}{}
	}
	return nil
}

func (m Message) Clone() Message {
	m.Text = strings.Clone(m.Text)
	m.Attachments = slices.Clone(m.Attachments)
	return m
}

// Equal reports whether two messages have the same complete authoring value.
// Attachment metadata participates because restored drafts and history must not
// silently retain a stale projection for an otherwise identical attachment ID.
func (m Message) Equal(other Message) bool {
	return m.Text == other.Text && slices.Equal(m.Attachments, other.Attachments)
}

// HasText reports whether the authored text contains semantic content. It does
// not normalize Text: leading indentation and trailing newlines belong to the
// user's prompt and must survive delivery unchanged.
func (m Message) HasText() bool { return strings.TrimSpace(m.Text) != "" }

// IsEmpty reports whether the message has neither semantic text nor an
// attachment. Whitespace next to attachments is not a separate text block.
func (m Message) IsEmpty() bool { return !m.HasText() && len(m.Attachments) == 0 }
