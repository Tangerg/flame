package runtimebinding

import (
	"crypto/sha256"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const maximumPaginationCursorBytes = protocol.MaximumPaginationCursorCharacters

// These requests have no continuation cursor, so accepting NextCursor would
// silently truncate the projection.
func requireCompletePage[T any](operation string, page *protocol.Page[T]) ([]T, error) {
	if page == nil {
		return nil, runtimeContractViolation("%s returned a nil page", operation)
	}
	if page.NextCursor != "" {
		return nil, runtimeContractViolation("%s returned an unusable continuation cursor", operation)
	}
	return page.Data, nil
}

func projectUniqueValues[Source any, Target any](
	operation string,
	values []Source,
	project func(Source) Target,
	identity func(Target) string,
) ([]Target, error) {
	return projectUniqueValuesFallible(operation, values, func(value Source) (Target, error) {
		return project(value), nil
	}, identity)
}

func projectUniqueValuesFallible[Source any, Target any](
	operation string,
	values []Source,
	project func(Source) (Target, error),
	identity func(Target) string,
) ([]Target, error) {
	projected := make([]Target, 0, len(values))
	for index, value := range values {
		row, err := project(value)
		if err != nil {
			return nil, runtimeContractViolation("%s item %d cannot be projected: %v", operation, index+1, err)
		}
		projected = append(projected, row)
	}
	if err := requireUniqueIdentities(operation, projected, identity); err != nil {
		return nil, err
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

func (c *cursorTraversal) cursor() string { return c.current }

func (c *cursorTraversal) advance(next string) (bool, error) {
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

func protocolPositiveInt(value int) *int {
	if value <= 0 {
		panic("runtimebinding: protocol positive integer must be greater than zero")
	}
	return &value
}

func protocolOptionalPositiveInt(value int) *int {
	if value == 0 {
		return nil
	}
	return protocolPositiveInt(value)
}
