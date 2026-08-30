package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/runidentity"
	"github.com/Tangerg/flame/cli/internal/sessionidentity"
)

const MaxMessageAttachments = 16

func (s StartRun) Validate() error {
	var problems []error
	if s.CommandID != "" {
		if err := s.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if _, err := sessionidentity.Parse(s.SessionID); err != nil {
		problems = append(problems, err)
	}
	if err := s.Message.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := s.Options.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("start run: %w", err)
	}
	return nil
}

func (d DeleteSession) Validate() error {
	var problems []error
	if d.CommandID != "" {
		if err := d.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if _, err := sessionidentity.Parse(d.SessionID); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (m Message) Validate() error {
	if strings.TrimSpace(m.Text) == "" && len(m.Attachments) == 0 {
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

func (s SubscribeRun) Validate() error {
	if _, err := runidentity.ParseRun(s.RunID); err != nil {
		return fmt.Errorf("subscribe run: %w", err)
	}
	if _, err := runidentity.ParseSegment(s.SegmentID); err != nil {
		return fmt.Errorf("subscribe run: %w", err)
	}
	if s.AfterEventID != "" {
		if _, err := runidentity.ParseEvent(s.AfterEventID); err != nil {
			return fmt.Errorf("subscribe run: after %w", err)
		}
	}
	return nil
}

func (r ResumeRun) Validate() error {
	if r.CommandID != "" {
		if err := r.CommandID.Validate(); err != nil {
			return fmt.Errorf("resume run: %w", err)
		}
	}
	if _, err := runidentity.ParseRun(r.RunID); err != nil {
		return fmt.Errorf("resume run: %w", err)
	}
	if len(r.Answers) == 0 {
		return errors.New("resume run: answers are empty")
	}
	seen := make(map[string]struct{}, len(r.Answers))
	for i, response := range r.Answers {
		if _, err := runidentity.ParseItem(response.ItemID); err != nil {
			return fmt.Errorf("resume run: answer %d: %w", i+1, err)
		}
		if response.Answer == nil {
			return fmt.Errorf("resume run: answer %d is nil", i+1)
		}
		if _, duplicate := seen[response.ItemID]; duplicate {
			return fmt.Errorf("resume run: item %q is answered more than once", response.ItemID)
		}
		seen[response.ItemID] = struct{}{}
	}
	if r.Message != nil {
		if err := r.Message.Validate(); err != nil {
			return fmt.Errorf("resume run: %w", err)
		}
	}
	return nil
}

func (c CancelRun) Validate() error {
	if c.CommandID != "" {
		if err := c.CommandID.Validate(); err != nil {
			return fmt.Errorf("cancel run: %w", err)
		}
	}
	if _, err := runidentity.ParseRun(c.RunID); err != nil {
		return fmt.Errorf("cancel run: %w", err)
	}
	return nil
}

func (s SteerRun) Validate() error {
	var problems []error
	if s.CommandID != "" {
		if err := s.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if _, err := runidentity.ParseRun(s.RunID); err != nil {
		problems = append(problems, err)
	}
	if _, err := runidentity.ParseSegment(s.SegmentID); err != nil {
		problems = append(problems, err)
	}
	if err := s.Message.Validate(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("steer run: %w", err)
	}
	return nil
}

func (s SegmentStream) Validate() error {
	var problems []error
	if _, err := runidentity.ParseRun(s.RunID); err != nil {
		problems = append(problems, err)
	}
	if _, err := runidentity.ParseSegment(s.SegmentID); err != nil {
		problems = append(problems, err)
	}
	if s.HeadEventID != "" {
		if _, err := runidentity.ParseEvent(s.HeadEventID); err != nil {
			problems = append(problems, fmt.Errorf("head %w", err))
		}
	}
	if s.Events == nil {
		problems = append(problems, errors.New("event stream is nil"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("segment stream: %w", err)
	}
	return nil
}

// ValidateStart enforces the runs.start-specific response invariant: every
// accepted start creates and names its opening user item.
func (s SegmentStream) ValidateStart() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if _, err := runidentity.ParseItem(s.UserItemID); err != nil {
		return fmt.Errorf("start segment stream: user %w", err)
	}
	return nil
}

// ValidateResume enforces both the target identity and the runs.resume response
// union. UserItemID exists exactly when an optional continuation message was
// committed with the answers.
func (s SegmentStream) ValidateResume(runID string, message *Message) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if _, err := runidentity.ParseRun(runID); err != nil {
		return fmt.Errorf("resume segment stream: %w", err)
	}
	if s.RunID != runID {
		return fmt.Errorf("resume segment stream: run %q does not match %q", s.RunID, runID)
	}
	hasUserItem := s.UserItemID != ""
	if hasUserItem {
		if _, err := runidentity.ParseItem(s.UserItemID); err != nil {
			return fmt.Errorf("resume segment stream: user %w", err)
		}
	}
	if hasUserItem != (message != nil) {
		return errors.New("resume segment stream: user item id does not match input presence")
	}
	return nil
}

// ValidateSubscription enforces that rebinding an existing segment creates no
// user item of its own.
func (s SegmentStream) ValidateSubscription() error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.UserItemID != "" {
		return errors.New("subscription segment stream carries a user item id")
	}
	return nil
}
