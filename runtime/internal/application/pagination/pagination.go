// Package pagination owns the shared keyset-paging contract used by application
// reads. A token names the query it was minted for and the sort position it
// stopped at, so continuing a page is a bounded seek rather than a scan.
//
// The anchor is the previous page's last sort key, never an offset or an element
// id. An offset shifts when rows are inserted before it, and an id anchor has to
// be located — which means materializing the collection to search it, and
// failing outright once the anchored row is deleted. A sort-key anchor turns
// continuation into `WHERE key > anchor ORDER BY key LIMIT n`, which stays exact
// under concurrent writes and never loads a page it will not return.
//
// A token also carries the query namespace it belongs to. Without that, a cursor
// from one query or filter set silently reinterprets against another and pages skip
// or repeat rows; with it, a mismatched cursor is rejected and the caller starts
// over. That check is the integrity guarantee here — no secret is involved,
// because the runtime has no user boundary to defend across.
package pagination

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/opaquetoken"
	"github.com/Tangerg/flame/runtime/internal/cursorresource"
)

// ErrInvalidCursor reports a cursor that cannot continue this query: damaged,
// minted by an older format, or issued for a different namespace or filter set.
// All of those have one remedy — restart from the first page — so they are one
// sentinel rather than a taxonomy the caller would branch on identically.
var ErrInvalidCursor = errors.New("pagination: cursor cannot continue this query")

// ErrInvalidCursorMaterial reports an authority-side attempt to mint a cursor
// without the query identity or keyset anchor needed to continue it. It is an
// explicit construction failure, never a panic or an empty token that silently
// means "end of collection".
var ErrInvalidCursorMaterial = errors.New("pagination: cursor material is invalid")

// ErrCursorTooLarge reports a continuation that exceeds Flame's finite cursor
// resource envelope. It applies symmetrically to received and newly minted
// cursors, so neither direction can hide an unbounded allocation contract.
var ErrCursorTooLarge = errors.New("pagination: cursor exceeds maximum size")

// ErrInvalidLimit reports a page size a read will not serve. Separate from
// ErrInvalidCursor because the caller's fix differs: correct the request, rather
// than start the collection over.
var ErrInvalidLimit = errors.New("pagination: page limit is invalid")

// formatVersion changes when the token layout does, so a cursor in flight across
// an upgrade is rejected instead of decoded as something else.
const formatVersion = 2

// MaximumCursorCharacters is Application's projection of the shared cursor
// resource envelope. It is exported so architecture tests can prove that every
// cursor authority uses the same limit as the public wire contract.
const MaximumCursorCharacters = cursorresource.MaximumCharacters

// Page is one keyset page: the rows, and the token that continues after them.
// An empty NextCursor means the page reached the end of the collection — the
// caller returns it as-is and never truncates a page silently.
type Page[T any] struct {
	Rows       []T
	NextCursor string
}

// token is the decoded cursor. Namespace and Filters identify the query; Key is
// the sort position the previous page ended at.
type token struct {
	Version   int      `json:"v"`
	Namespace string   `json:"n"`
	Filters   []string `json:"f,omitempty"`
	Key       []string `json:"k"`
}

// Encode mints the cursor that continues namespace past key. filters are the
// query's normalized inputs — every value that changes which rows match or the
// order they arrive in, including the sort direction, since a cursor from an
// ascending page cannot continue a descending one.
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
	encoded, err := opaquetoken.Encode(token{
		Version: formatVersion, Namespace: namespace,
		Filters: slices.Clone(filters), Key: slices.Clone(key),
	}, MaximumCursorCharacters)
	if err != nil {
		if errors.Is(err, opaquetoken.ErrTooLarge) {
			return "", ErrCursorTooLarge
		}
		return "", fmt.Errorf("%w: encode: %v", ErrInvalidCursorMaterial, err)
	}
	return encoded, nil
}

// Decode returns the sort position cursor stopped at, for the same namespace and
// filters that minted it. An empty cursor is the first page and yields a nil key
// with no error.
func Decode(cursor, namespace string, filters []string) ([]string, error) {
	if namespace == "" {
		return nil, ErrInvalidCursor
	}
	if cursor == "" {
		return nil, nil
	}
	if len(cursor) > MaximumCursorCharacters {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCursor, ErrCursorTooLarge)
	}
	var decoded token
	if err := opaquetoken.Decode(cursor, MaximumCursorCharacters, &decoded); err != nil {
		return nil, ErrInvalidCursor
	}
	if decoded.Version != formatVersion || decoded.Namespace != namespace ||
		!slices.Equal(decoded.Filters, filters) || len(decoded.Key) == 0 {
		return nil, ErrInvalidCursor
	}
	return decoded.Key, nil
}

// rawMaterialFits rejects obviously oversized material before JSON/Base64 can
// allocate from it. The test is deliberately a lower bound: every source byte
// and every slice element consumes at least one decoded byte. Escaping can make
// the final token larger, which the exact encoded-size check catches afterward.
func rawMaterialFits(namespace string, groups ...[]string) bool {
	remaining := MaximumCursorCharacters
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

// RequestedLimit is the caller's page-size intent. The zero value is the named
// default intent: use the read's own ceiling. An explicit intent can only be
// constructed with a positive value, so Application code never receives a raw
// integer whose zero might mean either "omitted" or "unbounded".
type RequestedLimit struct {
	mode  requestedLimitMode
	value int
}

type requestedLimitMode uint8

const (
	requestedLimitDefault requestedLimitMode = iota
	requestedLimitExplicit
)

// DefaultLimit asks a read to use its owned page-size ceiling.
func DefaultLimit() RequestedLimit { return RequestedLimit{mode: requestedLimitDefault} }

// NewLimit constructs an explicit positive page-size request.
func NewLimit(value int) (RequestedLimit, error) {
	if value <= 0 {
		return RequestedLimit{}, fmt.Errorf("%w: must be greater than zero", ErrInvalidLimit)
	}
	return RequestedLimit{mode: requestedLimitExplicit, value: value}, nil
}

// Resolve applies the read's positive ceiling to this request. Explicit values
// larger than the ceiling are clamped, keeping store reads bounded without
// silently reinterpreting an invalid non-positive request as a default.
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

// PageOf splits an over-fetched row set into the page and its continuation.
// Reads ask their store for limit+1 rows: getting the extra one is how "there is
// more" is known without a second count query. nextKey derives the anchor from
// the last row the page actually returns.
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
