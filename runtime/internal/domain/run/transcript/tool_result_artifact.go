package transcript

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
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

// A rich Tool result keeps its structured value in the transcript even when
// model-visible text moves to a blob. Only a plain text replacement hydrates
// the transcript from that blob.
type ToolResultRef struct {
	ID        ToolResultID
	ModelOnly bool
}

func (r ToolResultRef) Validate() error { return r.ID.Validate() }

func (r ToolResultRef) ValidateResult(result *tool.Result) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if result == nil {
		return errors.New("transcript: offloaded Tool result is absent")
	}
	if _, textual := result.String(); !r.ModelOnly && !textual {
		return errors.New("transcript: offloaded Tool result preview is not text")
	}
	return nil
}

func (r ToolResultRef) HydrateResult(result *tool.Result, body string) (tool.Result, error) {
	if err := r.ValidateResult(result); err != nil {
		return tool.Result{}, err
	}
	if body == "" {
		return tool.Result{}, errors.New("transcript: offloaded Tool result body is absent")
	}
	if r.ModelOnly {
		return *result, nil
	}
	return tool.NewResult(body)
}

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
