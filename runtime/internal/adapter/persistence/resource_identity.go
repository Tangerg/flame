package persistence

import "fmt"

// validateResource requires a present identity at a SQLite boundary. A present
// identity was parsed where it entered, so the zero value is the only invalid
// one left: a corrupted in-memory record must not become a different durable
// key.
func validateResource(operation string, value interface{ Validate() error }) error {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	return nil
}
