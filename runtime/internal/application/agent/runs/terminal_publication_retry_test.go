package runs

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestUnknownEffectCommitRetryBacksOffAndCaps(t *testing.T) {
	retry := unknownEffectCommitRetry{}
	got := make([]time.Duration, 8)
	for index := range got {
		got[index] = retry.advance()
	}
	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
		3200 * time.Millisecond,
		5 * time.Second,
		5 * time.Second,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("retry schedule = %v, want %v", got, want)
	}
}

func TestUnknownEffectCommitRetryStopsWithOwnerCause(t *testing.T) {
	cause := errors.New("runtime owner retired")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)

	retry := unknownEffectCommitRetry{}
	if err := retry.wait(ctx); !errors.Is(err, cause) {
		t.Fatalf("retry wait error = %v, want owner cause", err)
	}
}
