package main

import (
	"errors"

	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/Tangerg/scope/tools/web"
)

func classify(err error) string {
	if errors.Is(err, errStoredSnapshot) {
		return ""
	}
	switch {
	case errors.Is(err, capture.ErrDocument):
		return "invalid_document"
	case errors.Is(err, capture.ErrRange):
		return "invalid_page_range"
	case errors.Is(err, capture.ErrNotFound):
		return "snapshot_not_found"
	case errors.Is(err, web.ErrUnsupportedFilter), errors.Is(err, web.ErrUnsupportedFormat):
		return "unsupported_request"
	default:
		return ""
	}
}
