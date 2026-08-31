package fileobservation

import "testing"

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
