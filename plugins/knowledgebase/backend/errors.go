package main

import (
	"errors"
	knowledge "github.com/Tangerg/flame/plugins/knowledgebase/internal/domain"
	"github.com/Tangerg/flame/qimu/portable"
)

func classify(err error) string {
	if errors.Is(err, errStoredDocument) {
		return ""
	}
	for _, failure := range []struct {
		cause error
		code  string
	}{
		{knowledge.ErrInvalidDocument, "invalid_document"}, {knowledge.ErrRevisionConflict, "revision_conflict"},
		{knowledge.ErrCapacity, "capacity"}, {knowledge.ErrNotFound, "not_found"},
		{knowledge.ErrRetired, "retired_document"},
		{knowledge.ErrInvalidIndex, "invalid_index"}, {knowledge.ErrEmbeddingSpace, "embedding_space_changed"},
		{portable.ErrInvocationConflict, "command_conflict"},
	} {
		if errors.Is(err, failure.cause) {
			return failure.code
		}
	}
	return ""
}
