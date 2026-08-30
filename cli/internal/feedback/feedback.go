// Package feedback defines user-authored quality signals attached to the
// current runtime conversation.
package feedback

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/runidentity"
	"github.com/Tangerg/flame/cli/internal/sessionidentity"
)

type Rating string

const (
	Positive Rating = "positive"
	Negative Rating = "negative"
)

func ParseRating(value string) (Rating, error) {
	rating := Rating(strings.TrimSpace(value))
	if err := rating.Validate(); err != nil {
		return "", err
	}
	return rating, nil
}

func (r Rating) Validate() error {
	if r != "" && r != Positive && r != Negative {
		return fmt.Errorf("feedback rating %q is invalid", r)
	}
	return nil
}

type Signal struct {
	SessionID string
	RunID     string
	ItemID    string
	Rating    Rating
	Text      string
}

func (s Signal) Validate() error {
	var problems []error
	if s.SessionID != "" {
		if _, err := sessionidentity.Parse(s.SessionID); err != nil {
			problems = append(problems, err)
		}
	}
	if s.RunID != "" {
		if _, err := runidentity.ParseRun(s.RunID); err != nil {
			problems = append(problems, err)
		}
	}
	if s.ItemID != "" {
		if _, err := runidentity.ParseItem(s.ItemID); err != nil {
			problems = append(problems, err)
		}
	}
	if err := s.Rating.Validate(); err != nil {
		problems = append(problems, err)
	}
	if s.Rating == "" && strings.TrimSpace(s.Text) == "" {
		problems = append(problems, errors.New("feedback requires a rating or text"))
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("feedback signal: %w", err)
	}
	return nil
}

type Service interface {
	Record(context.Context, Signal) error
}
