package protocol

import flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"

const MaximumPaginationCursorCharacters = flameidentity.MaximumCursorCharacters

type PageQuery struct {
	Limit  *int   `json:"limit,omitzero"`
	Cursor string `json:"cursor,omitempty"`
}

// Go reflection cannot name an uninstantiated generic type, so the continuation
// constraint lives on this embedded type rather than on each Page instantiation.
type PageContinuation struct {
	NextCursor string `json:"nextCursor,omitempty"`
}

type Page[T any] struct {
	Data []T `json:"data"`
	PageContinuation
}

func NewPage[T any](data []T) *Page[T] { return NewPageWithCursor(data, "") }

// Rows are normalized to an empty slice because a nil slice marshals to null.
func NewPageWithCursor[T any](data []T, nextCursor string) *Page[T] {
	if data == nil {
		data = []T{}
	}
	return &Page[T]{
		Data:             data,
		PageContinuation: PageContinuation{NextCursor: nextCursor},
	}
}
