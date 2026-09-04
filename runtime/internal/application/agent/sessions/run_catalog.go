package sessions

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type sessionRunCatalogReader interface {
	ListRuns(ctx context.Context, sessionID string) ([]run.Run, error)
}

func listSessionRuns(
	ctx context.Context,
	reader sessionRunCatalogReader,
	sessionID string,
) ([]run.Run, error) {
	values, err := reader.ListRuns(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := validateRunCatalog(values, sessionID); err != nil {
		return nil, err
	}
	if err := validateRunAdmissionOrder(values); err != nil {
		return nil, err
	}
	return values, nil
}

func validateRunCatalog(values []run.Run, sessionID string) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		var err error
		if sessionID == "" {
			err = value.Validate()
		} else {
			err = value.ValidateForSession(sessionID)
		}
		if err != nil {
			return fmt.Errorf("sessions: Run store row %d is invalid: %w", index+1, err)
		}
		if _, duplicate := seen[value.ID()]; duplicate {
			return fmt.Errorf("sessions: Run store repeats %q", value.ID())
		}
		seen[value.ID()] = struct{}{}
	}
	return nil
}

func validateRunAdmissionOrder(values []run.Run) error {
	for index := 1; index < len(values); index++ {
		previous, current := values[index-1], values[index]
		if current.CreatedAt().After(previous.CreatedAt()) ||
			current.CreatedAt().Equal(previous.CreatedAt()) && current.ID() > previous.ID() {
			continue
		}
		return fmt.Errorf("sessions: Run %q is out of admission order after %q", current.ID(), previous.ID())
	}
	return nil
}
