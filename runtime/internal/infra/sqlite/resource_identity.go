package sqlite

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// SQLite methods are adapter boundaries: even when an Application aggregate
// was valid when planned, a corrupted in-memory record must not become a
// different durable key. These helpers retain the Domain's exact, bounded
// resource policy without normalizing the caller's value.
func validateSessionResource(operation, value string) error {
	if err := resourceid.ValidateSession(value); err != nil {
		return fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	return nil
}

func validateOptionalSessionResource(operation, value string) error {
	if value == "" {
		return nil
	}
	return validateSessionResource(operation, value)
}

func validateRunResource(operation, value string) error {
	if err := resourceid.ValidateRun(value); err != nil {
		return fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	return nil
}

func validateOptionalRunResource(operation, value string) error {
	if value == "" {
		return nil
	}
	return validateRunResource(operation, value)
}

func validateSegmentResource(operation, value string) error {
	if err := resourceid.ValidateSegment(value); err != nil {
		return fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	return nil
}

func validateItemResource(operation, value string) error {
	if err := resourceid.ValidateItem(value); err != nil {
		return fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	return nil
}
