package prompt

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/runtime/protocol"
)

// SteerRun injects an instruction only into the exact running segment the user
// is currently observing. A stale segment must be rejected, never retargeted.
type SteerRun struct {
	CommandID replay.CommandID
	RunID     string
	SegmentID string
	Message   Message
	Input     []protocol.ContentBlock `json:"-"`
}

type StartRun struct {
	CommandID replay.CommandID
	SessionID string
	Message   Message
	Options   RunOptions
	Input     []protocol.ContentBlock `json:"-"`
}

func (s StartRun) Validate() error {
	var problems []error
	if s.CommandID != "" {
		if err := s.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if err := protocol.ValidateSessionID(s.SessionID); err != nil {
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

func (s SteerRun) Validate() error {
	var problems []error
	if s.CommandID != "" {
		if err := s.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if err := protocol.ValidateRunID(s.RunID); err != nil {
		problems = append(problems, err)
	}
	if err := protocol.ValidateSegmentID(s.SegmentID); err != nil {
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

func (s SteerRun) Clone() SteerRun {
	s.Message = s.Message.Clone()
	s.Input = slices.Clone(s.Input)
	return s
}

func (s SteerRun) Equal(other SteerRun) bool {
	return s.CommandID == other.CommandID && s.RunID == other.RunID &&
		s.SegmentID == other.SegmentID && s.Message.Equal(other.Message) && slices.Equal(s.Input, other.Input)
}

func (s StartRun) Clone() StartRun {
	s.Message = s.Message.Clone()
	s.Options = s.Options.Clone()
	s.Input = slices.Clone(s.Input)
	return s
}

func (s StartRun) Equal(other StartRun) bool {
	return s.CommandID == other.CommandID && s.SessionID == other.SessionID &&
		s.Message.Equal(other.Message) && s.Options.Equal(other.Options) && slices.Equal(s.Input, other.Input)
}
