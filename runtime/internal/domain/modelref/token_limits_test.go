package modelref

import (
	"errors"
	"testing"
)

func tokenPointer(value int64) *int64 { return &value }

func TestTokenLimitsInputCeilingOwnsIndependentProviderFacts(t *testing.T) {
	tests := []struct {
		name        string
		values      TokenLimitValues
		reservation *int64
		wantCeiling int64
		wantKnown   bool
		wantErr     error
	}{
		{name: "unknown"},
		{
			name: "provider input maximum",
			values: TokenLimitValues{
				ContextWindow: tokenPointer(400_000), MaxInputTokens: tokenPointer(272_000), MaxOutputTokens: tokenPointer(128_000),
			},
			wantCeiling: 272_000, wantKnown: true,
		},
		{
			name: "explicit output reservation",
			values: TokenLimitValues{
				ContextWindow: tokenPointer(16_384), MaxOutputTokens: tokenPointer(8_192),
			},
			reservation: tokenPointer(8_192), wantCeiling: 8_192, wantKnown: true,
		},
		{
			name: "reservation tighter than independent input maximum",
			values: TokenLimitValues{
				ContextWindow: tokenPointer(400_000), MaxInputTokens: tokenPointer(272_000), MaxOutputTokens: tokenPointer(272_000),
			},
			reservation: tokenPointer(272_000), wantCeiling: 128_000, wantKnown: true,
		},
		{
			name: "requested output above provider maximum",
			values: TokenLimitValues{
				ContextWindow: tokenPointer(400_000), MaxInputTokens: tokenPointer(272_000), MaxOutputTokens: tokenPointer(128_000),
			},
			reservation: tokenPointer(128_001), wantErr: ErrOutputTokenLimitExceeded,
		},
		{
			name:        "requested output consumes total context",
			values:      TokenLimitValues{ContextWindow: tokenPointer(16_384)},
			reservation: tokenPointer(16_384), wantErr: ErrOutputReservationExhaustsContext,
		},
		{
			name: "streaming output larger than input context is independent",
			values: TokenLimitValues{
				ContextWindow: tokenPointer(16_384), MaxOutputTokens: tokenPointer(32_768),
			},
			reservation: tokenPointer(24_000), wantKnown: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits, err := NewTokenLimits(tt.values)
			if err != nil {
				t.Fatal(err)
			}
			reservation := OutputReservation{}
			if tt.reservation != nil {
				reservation, err = NewOutputReservation(*tt.reservation)
				if err != nil {
					t.Fatal(err)
				}
			}
			ceiling, known, err := limits.InputCeiling(reservation)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("InputCeiling() error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (ceiling != tt.wantCeiling || known != tt.wantKnown) {
				t.Fatalf(
					"InputCeiling() = (%d,%t), want (%d,%t)",
					ceiling,
					known,
					tt.wantCeiling,
					tt.wantKnown,
				)
			}
		})
	}
}

func TestTokenLimitsPreservesPresenceWithoutNumericSentinels(t *testing.T) {
	unknown, err := NewTokenLimits(TokenLimitValues{})
	if err != nil || !unknown.Unknown() {
		t.Fatalf("unknown limits = (%+v,%v), want explicit unknown", unknown, err)
	}
	if _, known := unknown.ContextWindow(); known {
		t.Fatal("unknown context window reported present")
	}

	limits, err := NewTokenLimits(TokenLimitValues{MaxInputTokens: tokenPointer(32_000)})
	if err != nil {
		t.Fatal(err)
	}
	if limits.Unknown() {
		t.Fatal("partial published facts reported unknown")
	}
	if value, known := limits.MaxInputTokens(); !known || value != 32_000 {
		t.Fatalf("max input = (%d,%t), want (32000,true)", value, known)
	}
	if _, known := limits.ContextWindow(); known {
		t.Fatal("absent context window reported present")
	}
}

func TestTokenLimitsRejectsSentinelsAndImpossiblePublishedMaximum(t *testing.T) {
	tests := []TokenLimitValues{
		{ContextWindow: tokenPointer(0)},
		{MaxInputTokens: tokenPointer(-1)},
		{ContextWindow: tokenPointer(1_000), MaxInputTokens: tokenPointer(1_001)},
	}
	for _, values := range tests {
		if _, err := NewTokenLimits(values); !errors.Is(err, ErrInvalidTokenLimits) {
			t.Fatalf("NewTokenLimits(%+v) error = %v, want ErrInvalidTokenLimits", values, err)
		}
	}
	for _, value := range []int64{0, -1} {
		if _, err := NewOutputReservation(value); !errors.Is(err, ErrInvalidOutputReservation) {
			t.Fatalf("NewOutputReservation(%d) error = %v, want ErrInvalidOutputReservation", value, err)
		}
	}
}
