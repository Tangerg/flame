package runs

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
)

const maximumReplayEpochBytes = 128

var (
	errMalformedReplayCursor   = errors.New("runs: replay cursor cannot be decoded")
	errReplayCursorTooLarge    = errors.New("runs: replay cursor exceeds maximum size")
	errReplaySequenceExhausted = errors.New("runs: replay sequence is exhausted")
)

type replayPosition struct {
	epoch     replayEpoch
	runID     identity.RunID
	segmentID identity.SegmentID
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

type encodedReplayPosition struct {
	Epoch     string             `json:"e"`
	RunID     identity.RunID     `json:"r"`
	SegmentID identity.SegmentID `json:"g"`
	Sequence  uint64             `json:"q"`
}

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
		return "", fmt.Errorf("%w: %w", errMalformedReplayCursor, err)
	}
	token, err := pagination.EncodeToken(encodedReplayPosition{
		Epoch: position.epoch.String(),
		RunID: position.runID, SegmentID: position.segmentID, Sequence: position.sequence,
	}, identity.MaximumCursorCharacters)
	if err != nil {
		if errors.Is(err, pagination.ErrTokenTooLarge) {
			return "", errReplayCursorTooLarge
		}
		return "", fmt.Errorf("%w: encode: %w", errMalformedReplayCursor, err)
	}
	return token, nil
}

func decodeReplayCursor(token string) (replayPosition, error) {
	if len(token) > identity.MaximumCursorCharacters {
		return replayPosition{}, fmt.Errorf("%w: %w", errMalformedReplayCursor, errReplayCursorTooLarge)
	}
	var encoded encodedReplayPosition
	if err := pagination.DecodeToken(token, identity.MaximumCursorCharacters, &encoded); err != nil {
		return replayPosition{}, errMalformedReplayCursor
	}
	epoch, err := parseReplayEpoch(encoded.Epoch)
	if err != nil {
		return replayPosition{}, fmt.Errorf("%w: %w", errMalformedReplayCursor, err)
	}
	runID := encoded.RunID
	if err := encoded.RunID.Validate(); err != nil {
		return replayPosition{}, fmt.Errorf("%w: %w", errMalformedReplayCursor, err)
	}
	segmentID := encoded.SegmentID
	if err := encoded.SegmentID.Validate(); err != nil {
		return replayPosition{}, fmt.Errorf("%w: %w", errMalformedReplayCursor, err)
	}
	position := replayPosition{epoch: epoch, runID: runID, segmentID: segmentID, sequence: encoded.Sequence}
	if err := position.validate(); err != nil {
		return replayPosition{}, fmt.Errorf("%w: %w", errMalformedReplayCursor, err)
	}
	return position, nil
}

func validateReplayEpochText(epoch string) error {
	if err := identity.ValidateURISafeASCII(epoch, maximumReplayEpochBytes); err != nil {
		return fmt.Errorf("replay epoch %w", err)
	}
	return nil
}

func validateReplayScope(scope streamScope) error {
	_, err := encodeReplayCursor(replayPosition{
		epoch: scope.Epoch, runID: scope.RunID, segmentID: scope.SegmentID,
		sequence: math.MaxUint64,
	})
	return err
}
