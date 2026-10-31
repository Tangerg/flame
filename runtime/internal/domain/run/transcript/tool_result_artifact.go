package transcript

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
)

// ToolResultIDPattern is the public wire grammar shared with generated
// protocol validators for portable offloaded-result records, and the only
// statement of it: uppercase unpadded base32, 2 to 64 characters.
const ToolResultIDPattern = `^[A-Z2-7]{2,64}$`

var toolResultIDExpression = regexp.MustCompile(ToolResultIDPattern)

var (
	errInvalidToolResultID        = errors.New("transcript: invalid tool-result ID")
	ErrToolResultIdentityConflict = errors.New("transcript: tool-result identity conflict")
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

// Validate bounds imported or model-supplied identifiers by [ToolResultIDPattern].
func (i ToolResultID) Validate() error {
	if !toolResultIDExpression.MatchString(string(i)) {
		return fmt.Errorf("%w: %q does not match %s", errInvalidToolResultID, string(i), ToolResultIDPattern)
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
		errs = append(errs, fmt.Errorf("transcript: tool result: %w", err))
	}
	if s.Body == "" {
		errs = append(errs, errors.New("transcript: tool result body is required"))
	}
	return errors.Join(errs...)
}

// ToolResultBlob is the portable, session-owned record needed to restore both
// transcript reconstruction and deferred result reads on another database.
// The Item that offloaded it names it; a Blob does not name the Item back.
type ToolResultBlob struct {
	ToolResultStage
	CreatedAt time.Time
}

func (b ToolResultBlob) Validate() error {
	errs := []error{b.ToolResultStage.Validate()}
	if b.CreatedAt.IsZero() {
		errs = append(errs, errors.New("transcript: tool result creation time is required"))
	}
	if err := instant.Validate(b.CreatedAt); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
