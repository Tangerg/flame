// Package annotation preserves opaque author observations in the same read cut as session evidence.
package annotation

import (
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/qimu/jsonvalue"
)

type Subject struct{ SessionID, RunID, ItemID string }

type Entry struct {
	CommandID string
	Source    extension.AnnotationSource
	Subject   Subject
	Content   jsonvalue.Value
	CreatedAt time.Time
}

var ErrConflict = errors.New("annotation: command already produced another observation")

func (e Entry) Validate() error {
	if e.CommandID == "" || e.CreatedAt.IsZero() {
		return errors.New("annotation: evidence is incomplete")
	}
	if err := e.Source.Validate(); err != nil {
		return err
	}
	if e.Subject.SessionID != "" {
		if err := identity.ValidateSession(e.Subject.SessionID); err != nil {
			return err
		}
	}
	if e.Subject.RunID != "" {
		if err := identity.ValidateRun(e.Subject.RunID); err != nil {
			return err
		}
	}
	if e.Subject.ItemID != "" {
		if err := identity.ValidateItem(e.Subject.ItemID); err != nil {
			return err
		}
	}
	return errors.Join(e.Content.Validate(), instant.Validate(e.CreatedAt))
}
