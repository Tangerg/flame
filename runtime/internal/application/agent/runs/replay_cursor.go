package runs

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/flame/runtime/internal/application/opaquetoken"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// replayCursorFormat changes when the token layout changes. A cursor held
// across an incompatible upgrade is refused instead of being misread.
const replayCursorFormat = 1

// maximumReplayEpochBytes bounds the process-local authority carried inside
// every replay cursor. crypto/rand.Text currently emits 26 RFC 4648 base32
// bytes and may grow in a future Go release; this envelope leaves that room
// without accepting an unbounded decoded identity.
const maximumReplayEpochBytes = 128

var (
	errMalformedReplayCursor   = errors.New("runs: replay cursor cannot be decoded")
	errReplayCursorTooLarge    = errors.New("runs: replay cursor exceeds maximum size")
	errReplaySequenceExhausted = errors.New("runs: replay sequence is exhausted")
)

// MaximumReplayCursorCharacters is Application's projection of the shared
// opaque-cursor envelope. Run event framing is added outside Application.
const MaximumReplayCursorCharacters = runtimeidentity.MaximumCursorCharacters

// replayPosition is a point in one Run journal. It stays private because the
// journal is the only authority that may mint, interpret, or compare one.
type replayPosition struct {
	epoch     replayEpoch
	runID     resourceid.RunID
	segmentID resourceid.SegmentID
	sequence  uint64
}

func (r replayPosition) validate() error {
	if err := r.epoch.Validate(); err != nil {
		return err
	}
	if err := r.runID.Validate(); err != nil {
		return err
	}
	if err := r.segmentID.Validate(); err != nil {
		return err
	}
	if r.sequence == 0 {
		return errors.New("replay sequence is zero")
	}
	return nil
}

// encodedReplayPosition is the versioned token payload. The compact field names
// reduce the cost of carrying a cursor on every published event; callers still
// treat the resulting token as opaque.
type encodedReplayPosition struct {
	Version   int    `json:"v"`
	Epoch     string `json:"e"`
	RunID     string `json:"r"`
	SegmentID string `json:"g"`
	Sequence  uint64 `json:"q"`
}

// newReplayEpoch mints the identity shared by every journal owned by one
// Coordinator instance. Randomness prevents a restart from accepting a stale
// cursor as current.
type replayEpoch struct{ value string }

func newReplayEpoch() replayEpoch { return replayEpoch{value: rand.Text()} }

func parseReplayEpoch(raw string) (replayEpoch, error) {
	if err := validateReplayEpochText(raw); err != nil {
		return replayEpoch{}, err
	}
	return replayEpoch{value: raw}, nil
}

func (r replayEpoch) Validate() error { return validateReplayEpochText(r.value) }

func (r replayEpoch) String() string { return r.value }

func encodeReplayCursor(position replayPosition) (string, error) {
	if err := position.validate(); err != nil {
		return "", fmt.Errorf("%w: %v", errMalformedReplayCursor, err)
	}
	token, err := opaquetoken.Encode(encodedReplayPosition{
		Version: replayCursorFormat, Epoch: position.epoch.String(),
		RunID: position.runID.String(), SegmentID: position.segmentID.String(), Sequence: position.sequence,
	}, MaximumReplayCursorCharacters)
	if err != nil {
		if errors.Is(err, opaquetoken.ErrTooLarge) {
			return "", errReplayCursorTooLarge
		}
		return "", fmt.Errorf("%w: encode: %v", errMalformedReplayCursor, err)
	}
	return token, nil
}

func decodeReplayCursor(token string) (replayPosition, error) {
	if len(token) > MaximumReplayCursorCharacters {
		return replayPosition{}, fmt.Errorf("%w: %w", errMalformedReplayCursor, errReplayCursorTooLarge)
	}
	var encoded encodedReplayPosition
	if err := opaquetoken.Decode(token, MaximumReplayCursorCharacters, &encoded); err != nil {
		return replayPosition{}, errMalformedReplayCursor
	}
	epoch, err := parseReplayEpoch(encoded.Epoch)
	if err != nil {
		return replayPosition{}, fmt.Errorf("%w: %v", errMalformedReplayCursor, err)
	}
	runID, err := resourceid.ParseRun(encoded.RunID)
	if err != nil {
		return replayPosition{}, fmt.Errorf("%w: %v", errMalformedReplayCursor, err)
	}
	segmentID, err := resourceid.ParseSegment(encoded.SegmentID)
	if err != nil {
		return replayPosition{}, fmt.Errorf("%w: %v", errMalformedReplayCursor, err)
	}
	position := replayPosition{epoch: epoch, runID: runID, segmentID: segmentID, sequence: encoded.Sequence}
	if encoded.Version != replayCursorFormat {
		return replayPosition{}, errMalformedReplayCursor
	}
	if err := position.validate(); err != nil {
		return replayPosition{}, fmt.Errorf("%w: %v", errMalformedReplayCursor, err)
	}
	return position, nil
}

func validateReplayEpochText(epoch string) error {
	if len(epoch) == 0 || len(epoch) > maximumReplayEpochBytes {
		return fmt.Errorf("replay epoch must contain 1 to %d URI-safe ASCII bytes", maximumReplayEpochBytes)
	}
	for index := range len(epoch) {
		character := epoch[index]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' || character == '.' || character == ':' {
			continue
		}
		return fmt.Errorf("replay epoch must contain 1 to %d URI-safe ASCII bytes", maximumReplayEpochBytes)
	}
	return nil
}

// validateReplayScope proves at journal construction that every cursor this
// scope can mint fits the resource envelope. MaxUint64 is the longest sequence
// spelling, so every earlier position is covered by this one admission.
func validateReplayScope(scope streamScope) error {
	_, err := encodeReplayCursor(replayPosition{
		epoch: scope.Epoch, runID: scope.RunID, segmentID: scope.SegmentID,
		sequence: math.MaxUint64,
	})
	return err
}
