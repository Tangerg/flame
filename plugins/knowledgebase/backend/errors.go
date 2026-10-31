package main

import (
	"errors"

	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
)

func classify(err error) (string, bool) {
	if errors.Is(err, errStoredDocument) {
		return "", false
	}
	for _, failure := range []struct {
		cause error
		code  string
	}{
		{knowledge.ErrInvalidDocument, "invalid_document"}, {knowledge.ErrRevisionConflict, "revision_conflict"},
		{knowledge.ErrCapacity, "capacity"}, {knowledge.ErrNotFound, "not_found"},
		{knowledge.ErrRetired, "retired_document"},
		{knowledge.ErrInvalidIndex, "invalid_index"}, {knowledge.ErrEmbeddingSpace, "embedding_space_changed"},
	} {
		if errors.Is(err, failure.cause) {
			return failure.code, true
		}
	}
	return "", false
}
