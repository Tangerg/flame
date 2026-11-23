// Tokens anchor on the previous page's last sort key, never an offset or element
// id: an offset shifts under inserts, and an id anchor must be located and fails
// once its row is deleted. A token also carries its query namespace so a cursor
// from another query or filter set is rejected instead of skipping or repeating
// rows.
package pagination

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

var ErrInvalidCursor = errors.New("pagination: cursor cannot continue this query")

var ErrInvalidCursorMaterial = errors.New("pagination: cursor material is invalid")

var ErrCursorTooLarge = errors.New("pagination: cursor exceeds maximum size")

var ErrInvalidLimit = errors.New("pagination: page limit is invalid")

type Page[T any] struct {
	Rows       []T
	NextCursor string
}

type token struct {
	Namespace string   `json:"n"`
	Filters   []string `json:"f,omitempty"`
	Key       []string `json:"k"`
}

// filters include the sort direction, since a cursor from an ascending page cannot
// continue a descending one.
func Encode(namespace string, filters []string, key []string) (string, error) {
	if namespace == "" {
		return "", fmt.Errorf("%w: namespace is required", ErrInvalidCursorMaterial)
	}
	if len(key) == 0 {
		return "", fmt.Errorf("%w: key is required", ErrInvalidCursorMaterial)
	}
	if !rawMaterialFits(namespace, filters, key) {
		return "", ErrCursorTooLarge
	}
	encoded, err := EncodeToken(token{
		Namespace: namespace,
		Filters:   slices.Clone(filters), Key: slices.Clone(key),
	}, identity.MaximumCursorCharacters)
	if err != nil {
		if errors.Is(err, ErrTokenTooLarge) {
			return "", ErrCursorTooLarge
		}
		return "", fmt.Errorf("%w: encode: %w", ErrInvalidCursorMaterial, err)
	}
	return encoded, nil
}

func Decode(cursor, namespace string, filters []string) ([]string, error) {
	if namespace == "" {
		return nil, ErrInvalidCursor
	}
	if cursor == "" {
		return nil, nil
	}
	if len(cursor) > identity.MaximumCursorCharacters {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCursor, ErrCursorTooLarge)
	}
	var decoded token
	if err := DecodeToken(cursor, identity.MaximumCursorCharacters, &decoded); err != nil {
		return nil, ErrInvalidCursor
	}
	if decoded.Namespace != namespace ||
		!slices.Equal(decoded.Filters, filters) || len(decoded.Key) == 0 {
		return nil, ErrInvalidCursor
	}
	return decoded.Key, nil
}

// The check is deliberately a lower bound; escaping can make the token larger, which
// the exact encoded-size check catches afterward.
func rawMaterialFits(namespace string, groups ...[]string) bool {
	remaining := identity.MaximumCursorCharacters
	consume := func(size int) bool {
		if size < 0 || size > remaining {
			return false
		}
		remaining -= size
		return true
	}
	if !consume(len(namespace)) {
		return false
	}
	for _, group := range groups {
		if !consume(len(group)) {
			return false
		}
		for _, value := range group {
			if !consume(len(value)) {
				return false
			}
		}
	}
	return true
}

type RequestedLimit struct {
	mode  requestedLimitMode
	value int
}

type requestedLimitMode uint8

const (
	requestedLimitDefault requestedLimitMode = iota
	requestedLimitExplicit
)

func DefaultLimit() RequestedLimit { return RequestedLimit{mode: requestedLimitDefault} }

func NewLimit(value int) (RequestedLimit, error) {
	if value <= 0 {
		return RequestedLimit{}, fmt.Errorf("%w: must be greater than zero", ErrInvalidLimit)
	}
	return RequestedLimit{mode: requestedLimitExplicit, value: value}, nil
}

func ParseLimit(value *int) (RequestedLimit, error) {
	if value == nil {
		return DefaultLimit(), nil
	}
	return NewLimit(*value)
}

func (l RequestedLimit) Resolve(ceiling int) (int, error) {
	if ceiling <= 0 {
		return 0, fmt.Errorf("%w: ceiling must be greater than zero", ErrInvalidLimit)
	}
	switch l.mode {
	case requestedLimitDefault:
		if l.value != 0 {
			return 0, fmt.Errorf("%w: default request carries a value", ErrInvalidLimit)
		}
		return ceiling, nil
	case requestedLimitExplicit:
		if l.value <= 0 {
			return 0, fmt.Errorf("%w: explicit request must be greater than zero", ErrInvalidLimit)
		}
		return min(l.value, ceiling), nil
	default:
		return 0, fmt.Errorf("%w: request mode is unknown", ErrInvalidLimit)
	}
}

func PageOf[T any](rows []T, limit int, namespace string, filters []string, nextKey func(T) []string) (Page[T], error) {
	if namespace == "" {
		return Page[T]{}, fmt.Errorf("%w: namespace is required", ErrInvalidCursorMaterial)
	}
	if limit <= 0 {
		return Page[T]{}, fmt.Errorf("%w: must be greater than zero", ErrInvalidLimit)
	}
	if len(rows) <= limit {
		return Page[T]{Rows: rows}, nil
	}
	if nextKey == nil {
		return Page[T]{}, fmt.Errorf("%w: next-key function is required", ErrInvalidCursorMaterial)
	}
	page := rows[:limit]
	nextCursor, err := Encode(namespace, filters, nextKey(page[len(page)-1]))
	if err != nil {
		return Page[T]{}, err
	}
	return Page[T]{
		Rows:       page,
		NextCursor: nextCursor,
	}, nil
}
