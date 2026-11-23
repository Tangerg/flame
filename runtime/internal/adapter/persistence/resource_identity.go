package persistence

import "fmt"

func validateResource(operation string, value interface{ Validate() error }) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	return nil
}
