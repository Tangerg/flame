package conversation

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

func (d DeleteSession) Validate() error {
	var problems []error
	if d.CommandID != "" {
		if err := d.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if err := runtimeprotocol.ValidateSessionID(d.SessionID); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s SubscribeRun) Validate() error {
	if s.Snapshot {
		if err := runtimeprotocol.ValidateSessionID(s.SessionID); err != nil {
			return fmt.Errorf("subscribe run snapshot: %w", err)
		}
		if s.AfterEventID != "" {
			return errors.New("subscribe run snapshot cannot replay a cursor")
		}
	}
	if err := runtimeprotocol.ValidateRunID(s.RunID); err != nil {
		return fmt.Errorf("subscribe run: %w", err)
	}
	if err := runtimeprotocol.ValidateSegmentID(s.SegmentID); err != nil {
		return fmt.Errorf("subscribe run: %w", err)
	}
	if s.AfterEventID != "" {
		if err := runtimeprotocol.ValidateRunEventID(s.AfterEventID); err != nil {
			return fmt.Errorf("subscribe run: after %w", err)
		}
	}
	return nil
}

func (r ResumeRun) Validate() error {
	if r.Message == nil && r.Input != nil {
		return errors.New("resume run: prepared input has no message")
	}
	if r.CommandID != "" {
		if err := r.CommandID.Validate(); err != nil {
			return fmt.Errorf("resume run: %w", err)
		}
	}
	if err := runtimeprotocol.ValidateRunID(r.RunID); err != nil {
		return fmt.Errorf("resume run: %w", err)
	}
	if len(r.Answers) == 0 {
		return errors.New("resume run: answers are empty")
	}
	seen := make(map[string]struct{}, len(r.Answers))
	for i, response := range r.Answers {
		if err := runtimeprotocol.ValidateItemID(response.ItemID); err != nil {
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
	if err := (runtimeprotocol.CancelRunRequest{RunID: c.RunID, Reason: c.Reason}).ValidateWire(); err != nil {
		return fmt.Errorf("cancel run: %w", err)
	}
	return nil
}

func (s SegmentStream) Validate() error {
	var problems []error
	if err := runtimeprotocol.ValidateRunID(s.RunID); err != nil {
		problems = append(problems, err)
	}
	if err := runtimeprotocol.ValidateSegmentID(s.SegmentID); err != nil {
		problems = append(problems, err)
	}
	if s.HeadEventID != "" {
		if err := runtimeprotocol.ValidateRunEventID(s.HeadEventID); err != nil {
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
	if err := runtimeprotocol.ValidateItemID(s.UserItemID); err != nil {
		return fmt.Errorf("start segment stream: user %w", err)
	}
	return nil
}

// ValidateResume enforces both the target identity and the runs.resume response
// union. UserItemID exists exactly when an optional continuation message was
// committed with the answers.
func (s SegmentStream) ValidateResume(runID string, message *prompt.Message) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := runtimeprotocol.ValidateRunID(runID); err != nil {
		return fmt.Errorf("resume segment stream: %w", err)
	}
	if s.RunID != runID {
		return fmt.Errorf("resume segment stream: run %q does not match %q", s.RunID, runID)
	}
	hasUserItem := s.UserItemID != ""
	if hasUserItem {
		if err := runtimeprotocol.ValidateItemID(s.UserItemID); err != nil {
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
