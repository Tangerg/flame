package reviews

import (
	"errors"
	"slices"
	"unicode/utf8"

	"github.com/invopop/jsonschema"
)

type Status string

const (
	Open             Status = "open"
	Resolved         Status = "resolved"
	MaxRevision      int64  = 1<<53 - 1
	ExampleReviewID         = "example-review"
	maxReviewIDBytes        = 128
)

var (
	ErrConflict          = errors.New("reviews: revision conflict")
	ErrRevisionExhausted = errors.New("reviews: revision exhausted")
)

type Review struct {
	id       string
	title    string
	status   Status
	revision int64
}

type Snapshot struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   Status `json:"status"`
	Revision int64  `json:"revision"`
}

func Restore(snapshot Snapshot) (Review, error) {
	if snapshot.ID != ExampleReviewID || snapshot.Title == "" || !validStatus(snapshot.Status) || snapshot.Revision < 1 || snapshot.Revision > MaxRevision {
		return Review{}, errors.New("reviews: invalid review")
	}
	return Review{id: snapshot.ID, title: snapshot.Title, status: snapshot.Status, revision: snapshot.Revision}, nil
}

func (r Review) Snapshot() Snapshot {
	return Snapshot{ID: r.id, Title: r.title, Status: r.status, Revision: r.revision}
}

func (r Review) Change(expectedRevision int64, status Status) (Review, error) {
	if err := (Update{ID: r.id, ExpectedRevision: expectedRevision, Status: status}).Validate(); err != nil {
		return Review{}, err
	}
	if expectedRevision != r.revision {
		return Review{}, ErrConflict
	}
	if r.revision == MaxRevision {
		return Review{}, ErrRevisionExhausted
	}
	r.status, r.revision = status, r.revision+1
	return r, nil
}

func (Status) JSONSchemaEnum() []string { return []string{string(Open), string(Resolved)} }

func validStatus(status Status) bool { return slices.Contains(status.JSONSchemaEnum(), string(status)) }

type Update struct {
	ID               string `json:"id"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Status           Status `json:"status"`
}

func (Update) JSONSchemaExtend(schema *jsonschema.Schema) {
	id, _ := schema.Properties.Get("id")
	// Schema lengths count characters. Admitted Unicode values that exceed the
	// byte bound remain business refusals with their own original receipt.
	id.MaxLength = new(uint64(maxReviewIDBytes))
}

func (u Update) Validate() error {
	if u.ID == "" || len(u.ID) > maxReviewIDBytes || !utf8.ValidString(u.ID) || u.ExpectedRevision < 1 || u.ExpectedRevision > MaxRevision || !validStatus(u.Status) {
		return errors.New("reviews: invalid update")
	}
	return nil
}

type Outcome string

const (
	Updated           Outcome = "updated"
	Conflict          Outcome = "revisionConflict"
	NotFound          Outcome = "notFound"
	Invalid           Outcome = "invalidUpdate"
	IdentityConflict  Outcome = "invocationConflict"
	RevisionExhausted Outcome = "revisionExhausted"
)

func (Outcome) JSONSchemaEnum() []string {
	return []string{string(Updated), string(Conflict), string(NotFound), string(Invalid), string(IdentityConflict), string(RevisionExhausted)}
}

type UpdateResult struct {
	Type   Outcome   `json:"type"`
	Review *Snapshot `json:"review,omitzero"`
}

func (r UpdateResult) Validate() error {
	switch r.Type {
	case Updated, Conflict:
		if r.Review == nil {
			return errors.New("reviews: outcome requires a review")
		}
		_, err := Restore(*r.Review)
		return err
	case NotFound, Invalid, IdentityConflict, RevisionExhausted:
		if r.Review == nil {
			return nil
		}
	}
	return errors.New("reviews: invalid update outcome")
}
