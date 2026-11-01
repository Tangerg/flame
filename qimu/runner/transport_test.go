package runner

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/qimu"
)

func TestControlFrameBudgetIncludesDelimiter(t *testing.T) {
	for _, delta := range []int{-1, 0, 1} {
		size := maxControlFrameBytes + delta
		value := strings.Repeat("x", size-len("\"\"\n"))
		var sent bytes.Buffer
		writeErr := writeFrame(&sent, value)
		wire := "\"" + value + "\"\n"
		received, readErr := readLine(bufio.NewReaderSize(strings.NewReader(wire), 31))
		if delta > 0 {
			if !errors.Is(writeErr, qimu.ErrCapacity) || !errors.Is(readErr, qimu.ErrCapacity) || sent.Len() != 0 {
				t.Fatalf("over budget: write=%v read=%v sent=%d", writeErr, readErr, sent.Len())
			}
		} else if writeErr != nil || readErr != nil || !bytes.Equal(sent.Bytes(), received) || sent.Len() != size {
			t.Fatalf("frame of %d bytes: write=%v read=%v sent=%d", size, writeErr, readErr, sent.Len())
		}
	}
}

func TestControlResponsePreservesInvalidReceipt(t *testing.T) {
	var encoded bytes.Buffer
	if err := writeResponse(&encoded, nil, qimu.ErrInvalid); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeResponse(encoded.Bytes()); !errors.Is(err, qimu.ErrInvalid) {
		t.Fatalf("invalid receipt lost its refusal: %v", err)
	}
}
