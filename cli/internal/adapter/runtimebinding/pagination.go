package runtimebinding

import (
	"crypto/sha256"
	"fmt"

	"github.com/Tangerg/flame/runtime/protocol"
)

// maximumPaginationCursorBytes projects the Runtime-owned public contract into
// the CLI adapter. Pagination cursors are ASCII, so characters and bytes are the
// same unit here; the CLI never owns a second numeric ceiling.
const maximumPaginationCursorBytes = protocol.MaximumPaginationCursorCharacters

// requireCompletePage protects adapters for list operations whose request has
// no continuation cursor. Accepting NextCursor there would silently truncate
// the CLI projection because the runtime offers no way to fetch the remainder.
func requireCompletePage[T any](operation string, page *protocol.Page[T]) ([]T, error) {
	if page == nil {
		return nil, runtimeContractViolation("%s returned a nil page", operation)
	}
	if page.NextCursor != "" {
		return nil, runtimeContractViolation("%s returned an unusable continuation cursor", operation)
	}
	return page.Data, nil
}

// validProjection is the shared behavior required of every catalog row after
// the wire value has been projected into the CLI's own model.
type validProjection interface {
	Validate() error
}

// projectUniqueValues projects, validates, and identity-checks one complete
// catalog. The operation returns no partial list: a malformed or repeated row
// makes the whole Runtime response a contract violation.
func projectUniqueValues[Source any, Target validProjection](
	operation string,
	values []Source,
	project func(Source) Target,
	identity func(Target) string,
) ([]Target, error) {
	return projectUniqueValuesFallible(operation, values, func(value Source) (Target, error) {
		return project(value), nil
	}, identity)
}

// projectUniqueValuesFallible is the strict variant for projections whose wire
// union can be malformed independently of the resulting model's validation.
func projectUniqueValuesFallible[Source any, Target validProjection](
	operation string,
	values []Source,
	project func(Source) (Target, error),
	identity func(Target) string,
) ([]Target, error) {
	projected := make([]Target, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		row, err := project(value)
		if err != nil {
			return nil, runtimeContractViolation("%s item %d cannot be projected: %v", operation, index+1, err)
		}
		if err := row.Validate(); err != nil {
			return nil, runtimeContractViolation("%s item %d is invalid: %v", operation, index+1, err)
		}
		key := identity(row)
		if _, duplicate := seen[key]; duplicate {
			return nil, runtimeContractViolation("%s repeats %q", operation, key)
		}
		seen[key] = struct{}{}
		projected = append(projected, row)
	}
	return projected, nil
}

type cursorTraversal struct {
	operation        string
	current          string
	pageRequests     int
	maximumRequests  int
	seenFingerprints map[[sha256.Size]byte]struct{}
}

func newCursorTraversal(operation, initial string, maximumRequests int) (*cursorTraversal, error) {
	if maximumRequests <= 0 {
		return nil, fmt.Errorf("%s pagination request capacity must be greater than zero", operation)
	}
	if err := validateRequestCursor(operation, initial); err != nil {
		return nil, err
	}
	traversal := &cursorTraversal{
		operation:        operation,
		current:          initial,
		pageRequests:     1,
		maximumRequests:  maximumRequests,
		seenFingerprints: make(map[[sha256.Size]byte]struct{}, maximumRequests),
	}
	if initial != "" {
		traversal.seenFingerprints[cursorFingerprint(initial)] = struct{}{}
	}
	return traversal, nil
}

func (c *cursorTraversal) Current() string { return c.current }

func (c *cursorTraversal) Advance(next string) (bool, error) {
	if next == "" {
		c.current = ""
		return false, nil
	}
	if len(next) > maximumPaginationCursorBytes {
		return false, runtimeContractViolation(
			"%s returned a continuation cursor larger than %d bytes",
			c.operation,
			maximumPaginationCursorBytes,
		)
	}
	fingerprint := cursorFingerprint(next)
	if _, exists := c.seenFingerprints[fingerprint]; exists {
		return false, runtimeContractViolation("%s returned a cyclic continuation cursor", c.operation)
	}
	if c.pageRequests >= c.maximumRequests {
		return false, runtimeContractViolation(
			"%s exceeded its %d-page traversal capacity",
			c.operation,
			c.maximumRequests,
		)
	}
	c.seenFingerprints[fingerprint] = struct{}{}
	c.pageRequests++
	c.current = next
	return true, nil
}

func cursorFingerprint(cursor string) [sha256.Size]byte { return sha256.Sum256([]byte(cursor)) }

func validateRequestCursor(operation, cursor string) error {
	if len(cursor) > maximumPaginationCursorBytes {
		return fmt.Errorf(
			"%s cursor exceeds the %d-byte transport limit",
			operation,
			maximumPaginationCursorBytes,
		)
	}
	return nil
}

func validateContinuationCursor(operation, request, next string) error {
	if len(next) > maximumPaginationCursorBytes {
		return runtimeContractViolation(
			"%s returned a continuation cursor larger than %d bytes",
			operation,
			maximumPaginationCursorBytes,
		)
	}
	if next != "" && next == request {
		return runtimeContractViolation("%s returned its request cursor as the continuation", operation)
	}
	return nil
}

// protocolPositiveInt owns pointer-backed optional-positive wire construction.
// Callers decide presence and units before reaching this boundary; this helper
// only prevents an invalid non-positive value from being serialized as present.
func protocolPositiveInt(value int) *int {
	if value <= 0 {
		panic("runtimebinding: protocol positive integer must be greater than zero")
	}
	return &value
}
