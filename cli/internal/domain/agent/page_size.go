package agent

import (
	"errors"
	"fmt"
)

const (
	// DefaultPageRows is the CLI product default shown by list commands.
	DefaultPageRows = 20
	// MaximumPageRows is the largest page the CLI asks any catalog to return.
	MaximumPageRows = 100
)

var ErrInvalidPageSize = errors.New("catalog page size is invalid")

type pageSizeKind uint8

const (
	defaultPageSize pageSizeKind = iota + 1
	explicitPageSize
)

// PageSize distinguishes the CLI's named product default from an explicit
// positive row count. Its zero value is invalid, so every catalog caller must
// choose its paging intent.
type PageSize struct {
	kind pageSizeKind
	rows int
}

func DefaultPageSize() PageSize { return PageSize{kind: defaultPageSize} }

func NewPageSize(rows int) (PageSize, error) {
	if rows <= 0 || rows > MaximumPageRows {
		return PageSize{}, fmt.Errorf("%w: rows must be between 1 and %d", ErrInvalidPageSize, MaximumPageRows)
	}
	return PageSize{kind: explicitPageSize, rows: rows}, nil
}

func MaximumPageSize() PageSize {
	return PageSize{kind: explicitPageSize, rows: MaximumPageRows}
}

// Rows resolves the named default to a concrete adapter request and rejects
// impossible internal states before any catalog I/O occurs.
func (p PageSize) Rows() (int, error) {
	switch p.kind {
	case defaultPageSize:
		if p.rows != 0 {
			return 0, fmt.Errorf("%w: default carries rows", ErrInvalidPageSize)
		}
		return DefaultPageRows, nil
	case explicitPageSize:
		if p.rows <= 0 || p.rows > MaximumPageRows {
			return 0, fmt.Errorf("%w: explicit rows must be between 1 and %d", ErrInvalidPageSize, MaximumPageRows)
		}
		return p.rows, nil
	default:
		return 0, fmt.Errorf("%w: kind is unknown", ErrInvalidPageSize)
	}
}
