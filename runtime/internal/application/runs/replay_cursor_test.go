package runs

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

func testReplayPosition() replayPosition {
	return replayPosition{
		epoch: testReplayEpoch("epoch_1"), runID: testRunResourceID("run_1"),
		segmentID: testSegmentResourceID("seg_1"), sequence: 42,
	}
}

func testReplayEpoch(raw string) replayEpoch {
	epoch, err := parseReplayEpoch(raw)
	if err != nil {
		panic(err)
	}
	return epoch
}

func testRunResourceID(raw string) resourceid.RunID {
	identity, err := resourceid.ParseRun(raw)
	if err != nil {
		panic(err)
	}
	return identity
}

func testSegmentResourceID(raw string) resourceid.SegmentID {
	identity, err := resourceid.ParseSegment(raw)
	if err != nil {
		panic(err)
	}
	return identity
}

func TestReplayCursorResourceEnvelopeRejectsBothDirections(t *testing.T) {
	oversized := strings.Repeat("x", MaximumReplayCursorCharacters+1)
	if _, err := decodeReplayCursor(oversized); !errors.Is(err, errMalformedReplayCursor) ||
		!errors.Is(err, errReplayCursorTooLarge) {
		t.Fatalf("decode oversized cursor err = %v, want malformed and too large", err)
	}
}

func TestReplayCursorEncodeRejectsInvalidPositionWithoutPanicking(t *testing.T) {
	if cursor, err := encodeReplayCursor(replayPosition{}); cursor != "" || !errors.Is(err, errMalformedReplayCursor) {
		t.Fatalf("encode zero position = (%q, %v), want empty and malformed", cursor, err)
	}
}

func TestReplayCursorRoundTrip(t *testing.T) {
	want := testReplayPosition()
	cursor, err := encodeReplayCursor(want)
	if err != nil {
		t.Fatalf("encode replay cursor: %v", err)
	}
	got, err := decodeReplayCursor(cursor)
	if err != nil {
		t.Fatalf("decode replay cursor: %v", err)
	}
	if got != want {
		t.Fatalf("replay position = %+v, want %+v", got, want)
	}
}

func TestReplayCursorRejectsMalformedPayloads(t *testing.T) {
	encodePayload := func(payload string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(payload))
	}
	tests := []struct {
		name  string
		token string
	}{
		{name: "empty", token: ""},
		{name: "not base64", token: "!!!!"},
		{name: "not json", token: encodePayload("hello")},
		{name: "wrong version", token: encodePayload(`{"v":99,"e":"epoch_1","r":"run_1","g":"seg_1","q":1}`)},
		{name: "missing epoch", token: encodePayload(`{"v":1,"e":"","r":"run_1","g":"seg_1","q":1}`)},
		{name: "epoch with whitespace", token: encodePayload(`{"v":1,"e":"epoch 1","r":"run_1","g":"seg_1","q":1}`)},
		{name: "epoch outside URI alphabet", token: encodePayload(`{"v":1,"e":"epoch/1","r":"run_1","g":"seg_1","q":1}`)},
		{name: "missing run", token: encodePayload(`{"v":1,"e":"epoch_1","r":"","g":"seg_1","q":1}`)},
		{name: "run with whitespace", token: encodePayload(`{"v":1,"e":"epoch_1","r":"run 1","g":"seg_1","q":1}`)},
		{name: "missing segment", token: encodePayload(`{"v":1,"e":"epoch_1","r":"run_1","g":"","q":1}`)},
		{name: "segment with control character", token: encodePayload(`{"v":1,"e":"epoch_1","r":"run_1","g":"seg\u000a1","q":1}`)},
		{name: "zero sequence", token: encodePayload(`{"v":1,"e":"epoch_1","r":"run_1","g":"seg_1","q":0}`)},
		{name: "unknown field", token: encodePayload(`{"v":1,"e":"epoch_1","r":"run_1","g":"seg_1","q":1,"x":true}`)},
		{name: "trailing value", token: encodePayload(`{"v":1,"e":"epoch_1","r":"run_1","g":"seg_1","q":1} {}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeReplayCursor(test.token); !errors.Is(err, errMalformedReplayCursor) {
				t.Fatalf("decode error = %v, want malformed replay cursor", err)
			}
		})
	}
}

func TestReplayEpochsDoNotRepeat(t *testing.T) {
	seen := make(map[string]bool, 64)
	for range 64 {
		epoch := newReplayEpoch()
		if err := epoch.Validate(); err != nil {
			t.Fatalf("generated epoch %q is invalid: %v", epoch, err)
		}
		if seen[epoch.String()] {
			t.Fatalf("epoch %q was minted twice", epoch)
		}
		seen[epoch.String()] = true
	}
}
