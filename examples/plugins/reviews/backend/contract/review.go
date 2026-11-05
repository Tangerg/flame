package reviews

import (
	"errors"
	"slices"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/exactint"
	"github.com/Tangerg/scope/core/jsonschema"
)

type Status string

const (
	Open             Status = "open"
	Resolved         Status = "resolved"
	ExampleReviewID         = "example-review"
	maxReviewIDBytes        = 128
)

type Snapshot struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   Status `json:"status"`
	Revision int64  `json:"revision"`
}

func (Status) JSONSchemaEnum() []string { return []string{string(Open), string(Resolved)} }

func validStatus(status Status) bool { return slices.Contains(status.JSONSchemaEnum(), string(status)) }

type Update struct {
	ID               string `json:"id"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Status           Status `json:"status"`
}

func (Update) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{Properties: map[string]jsonschema.Constraints{
		"id": {MaxLength: new(uint64(maxReviewIDBytes))},
	}}
}

// ValidateChange runs inside the receipt owner. Structurally admitted business
// refusals must retain the same original outcome through both command entrances.
func (u Update) ValidateChange() error {
	if u.ID == "" || len(u.ID) > maxReviewIDBytes || !utf8.ValidString(u.ID) || u.ExpectedRevision < 1 || u.ExpectedRevision > exactint.Maximum || !validStatus(u.Status) {
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
		return r.Review.Validate()
	case NotFound, Invalid, IdentityConflict, RevisionExhausted:
		if r.Review == nil {
			return nil
		}
	}
	return errors.New("reviews: invalid update outcome")
}

func (snapshot Snapshot) Validate() error {
	if snapshot.ID != ExampleReviewID || snapshot.Title == "" || !validStatus(snapshot.Status) || snapshot.Revision < 1 || snapshot.Revision > exactint.Maximum {
		return errors.New("reviews: invalid review")
	}
	return nil
}
