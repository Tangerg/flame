package skilllearning

import (
	"math"
	"testing"
	"time"
)

func TestArchiveAdvicePreservesAbsenceAndRejectsOverflow(t *testing.T) {
	if duration, err := (ArchiveAdvice{}).Duration(); err != nil || duration != 0 {
		t.Fatalf("absent advice = %v, %v", duration, err)
	}
	for _, value := range []int64{0, -1, math.MaxInt64/int64(time.Millisecond) + 1, math.MaxInt64} {
		if _, err := (ArchiveAdvice{AfterMillis: &value}).Duration(); err == nil {
			t.Fatalf("invalid duration %d was admitted", value)
		}
	}
	value := int64(30 * 24 * time.Hour / time.Millisecond)
	if duration, err := (ArchiveAdvice{AfterMillis: &value}).Duration(); err != nil || duration != 30*24*time.Hour {
		t.Fatalf("advice = %v, %v", duration, err)
	}
}
