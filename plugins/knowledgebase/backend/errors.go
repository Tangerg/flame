package main

import (
	"errors"

	"github.com/Tangerg/flame/plugins/knowledgebase/internal/knowledge"
)

var errHumanMove = errors.New("knowledge base: a person must move the material")

func classify(err error) (string, bool) {
	if errors.Is(err, errStoredDocument) {
		return "", false
	}
	for _, failure := range []struct {
		cause error
		code  string
	}{
		{errClaimInvalid, "invalid_claim"}, {errWorkInvalid, "invalid_work"}, {errHumanClaim, "not_allowed"}, {errHumanMove, "not_allowed"}, {knowledge.ErrInvalidDocument, "invalid_document"}, {knowledge.ErrRevisionConflict, "revision_conflict"},
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
