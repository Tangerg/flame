// Package annotation preserves opaque author observations in the same read cut as session evidence.
package annotation

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"

	framevalue "github.com/Tangerg/frame/value"
)

// Subject names what an observation is about. Each identity is optional, and a
// reference may outlive its target.
type Subject struct {
	SessionID identity.SessionID
	RunID     identity.RunID
	ItemID    identity.ItemID
}

type Entry struct {
	CommandID string
	Source    extension.AnnotationSource
	Subject   Subject
	Content   framevalue.JSON
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
	return errors.Join(e.Content.Validate(), instant.Validate(e.CreatedAt))
}
