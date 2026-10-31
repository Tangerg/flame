package main

import (
	"errors"

	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/Tangerg/scope/tools/web"
)

func classify(err error) (string, bool) {
	if errors.Is(err, errStoredSnapshot) {
		return "", false
	}
	switch {
	case errors.Is(err, capture.ErrDocument):
		return "invalid_document", true
	case errors.Is(err, capture.ErrRange):
		return "invalid_page_range", true
	case errors.Is(err, capture.ErrNotFound):
		return "snapshot_not_found", true
	case errors.Is(err, web.ErrUnsupportedFilter), errors.Is(err, web.ErrUnsupportedFormat):
		return "unsupported_request", true
	default:
		return "", false
	}
}
