package transcript

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
)

const (
	minIDLength = 2
	maxIDLength = 64
	// ToolResultIDPattern is the public wire grammar shared with generated
	// protocol validators for portable offloaded-result records.
	ToolResultIDPattern = `^[A-Z2-7]{2,64}$`
)

var (
	errInvalidToolResultID        = errors.New("toolresult: invalid tool-result ID")
	ErrToolResultIdentityConflict = errors.New("toolresult: tool-result identity conflict")
)

// ToolResultID is the opaque identity copied from an offload preview into
// later result reads and portable session exports.
type ToolResultID string

// ParseToolResultID validates raw before admitting it as an offloaded-result identity.
func ParseToolResultID(raw string) (ToolResultID, error) {
	id := ToolResultID(raw)
	if err := id.Validate(); err != nil {
		return "", err
	}
	return id, nil
}

func (i ToolResultID) String() string { return string(i) }

// Validate accepts uppercase unpadded base32 and bounds imported or
// model-supplied identifiers.
func (i ToolResultID) Validate() error {
	raw := string(i)
	if len(raw) < minIDLength || len(raw) > maxIDLength {
		return fmt.Errorf("%w: length must be between %d and %d characters", errInvalidToolResultID, minIDLength, maxIDLength)
	}
	for _, char := range raw {
		if (char < 'A' || char > 'Z') && (char < '2' || char > '7') {
			return fmt.Errorf("%w: %q is not uppercase base32", errInvalidToolResultID, raw)
		}
	}
	return nil
}

// ToolResultRef is the typed link carried with a transcript item after its full result
// has been moved to durable blob storage.
type ToolResultRef struct {
	ID ToolResultID
}

func (r ToolResultRef) Validate() error { return r.ID.Validate() }

// ToolResultStage is the complete unbound record persisted only after its
// rendered preview has proven worth evicting from model context.
type ToolResultStage struct {
	ID        ToolResultID
	SessionID string
	Body      string
}

func (s ToolResultStage) Validate() error {
	var errs []error
	if err := s.ID.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := identity.ValidateSession(s.SessionID); err != nil {
		errs = append(errs, fmt.Errorf("toolresult: %w", err))
	}
	if s.Body == "" {
		errs = append(errs, errors.New("toolresult: body is required"))
	}
	return errors.Join(errs...)
}

// ToolResultBlob is the portable, session-owned record needed to restore both
// transcript reconstruction and deferred result reads on another database.
// The Item that offloaded it names it; a Blob does not name the Item back.
type ToolResultBlob struct {
	ID        ToolResultID
	SessionID string
	Body      string
	CreatedAt time.Time
}

func (b ToolResultBlob) Validate() error {
	var errs []error
	if err := b.ID.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := identity.ValidateSession(b.SessionID); err != nil {
		errs = append(errs, fmt.Errorf("toolresult: %w", err))
	}
	if b.Body == "" {
		errs = append(errs, errors.New("toolresult: body is required"))
	}
	if b.CreatedAt.IsZero() {
		errs = append(errs, errors.New("toolresult: creation time is required"))
	}
	if err := instant.Validate(b.CreatedAt); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
