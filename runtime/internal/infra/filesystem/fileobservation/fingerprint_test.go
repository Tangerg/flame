package fileobservation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFingerprintEncoderPreservesFieldBoundaries(t *testing.T) {
	left := newFingerprintEncoder()
	left.field(fingerprintFieldLogicalPath, "a")
	left.field(fingerprintFieldPhysicalPath, "bc")

	right := newFingerprintEncoder()
	right.field(fingerprintFieldLogicalPath, "ab")
	right.field(fingerprintFieldPhysicalPath, "c")

	if left.sum() == right.sum() {
		t.Fatal("different field boundaries produced the same fingerprint")
	}
}

func TestFingerprintEncoderBoundsContentToObservedSize(t *testing.T) {
	const expectedSize = int64(8)
	input := strings.NewReader(strings.Repeat("x", 1<<20))
	encoder := newFingerprintEncoder()

	if err := encoder.content(input, expectedSize); err == nil {
		t.Fatal("growing content was accepted")
	}
	if consumed := (1 << 20) - input.Len(); consumed != int(expectedSize)+1 {
		t.Fatalf("consumed = %d bytes, want %d", consumed, expectedSize+1)
	}
	if err := newFingerprintEncoder().content(strings.NewReader("short"), expectedSize); err == nil {
		t.Fatal("shrinking content was accepted")
	}
	if err := newFingerprintEncoder().content(strings.NewReader("12345678"), expectedSize); err != nil {
		t.Fatalf("stable content: %v", err)
	}
}

func TestVerifyObservedFileVersionRejectsMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observed")
	if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	before, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after mutation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyObservedFileVersion(nil, path, file, before); err == nil {
		t.Fatal("mutated file version was accepted")
	}
}
