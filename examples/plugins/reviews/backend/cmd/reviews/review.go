package main

import (
	"errors"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
)

var (
	errConflict          = errors.New("reviews: revision conflict")
	errRevisionExhausted = errors.New("reviews: revision exhausted")
)

type review struct {
	id       string
	title    string
	status   reviews.Status
	revision int64
}

func restoreReview(snapshot reviews.Snapshot) (review, error) {
	if err := snapshot.Validate(); err != nil {
		return review{}, err
	}
	return review{id: snapshot.ID, title: snapshot.Title, status: snapshot.Status, revision: snapshot.Revision}, nil
}

func (r review) snapshot() reviews.Snapshot {
	return reviews.Snapshot{ID: r.id, Title: r.title, Status: r.status, Revision: r.revision}
}

func (r review) change(expectedRevision int64, status reviews.Status) (review, error) {
	if err := (reviews.Update{ID: r.id, ExpectedRevision: expectedRevision, Status: status}).Validate(); err != nil {
		return review{}, err
	}
	if expectedRevision != r.revision {
		return review{}, errConflict
	}
	if r.revision == exactint.Maximum {
		return review{}, errRevisionExhausted
	}
	r.status, r.revision = status, r.revision+1
	return r, nil
}
