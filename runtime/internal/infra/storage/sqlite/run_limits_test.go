package sqlite

import (
	"testing"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestRunLimitsStoredShapeDistinguishesUnlimitedAndPresentCaps(t *testing.T) {
	unlimited := runLimitsRowOf(rundomain.UnlimitedLimits())
	if unlimited.Type != runLimitsUnlimited || unlimited.MaxTotalTokens != nil || unlimited.MaxSteps != nil || unlimited.MaxBudgetUSD != nil {
		t.Fatalf("unlimited row = %+v", unlimited)
	}
	if decoded, err := runLimitsFromStored(unlimited.Type, unlimited.MaxTotalTokens, unlimited.MaxSteps, unlimited.MaxBudgetUSD); err != nil || !decoded.Unlimited() {
		t.Fatalf("decode unlimited = %+v, %v", decoded, err)
	}

	limited := testsupport.MustRunLimits(rundomain.LimitValues{MaxSteps: testsupport.Pointer(7)})
	row := runLimitsRowOf(limited)
	if row.Type != runLimitsLimited || row.MaxTotalTokens != nil || row.MaxSteps == nil || *row.MaxSteps != 7 || row.MaxBudgetUSD != nil {
		t.Fatalf("limited row = %+v", row)
	}
	if decoded, err := runLimitsFromStored(row.Type, row.MaxTotalTokens, row.MaxSteps, row.MaxBudgetUSD); err != nil || decoded != limited {
		t.Fatalf("decode limited = %+v, %v; want %+v", decoded, err, limited)
	}
}

func TestRunAccountingCodecRejectsOldSentinelsAndMalformedPolicies(t *testing.T) {
	for name, encoded := range map[string]string{
		"old flat zero fields": `{"steps":1,"maxTotalTokens":0,"maxSteps":0,"maxBudgetUsd":0}`,
		"limited zero":         `{"steps":1,"limits":{"type":"limited","maxSteps":0}}`,
		"limited empty":        `{"steps":1,"limits":{"type":"limited"}}`,
		"unlimited with cap":   `{"steps":1,"limits":{"type":"unlimited","maxSteps":1}}`,
		"unknown type":         `{"steps":1,"limits":{"type":"sometimes"}}`,
		"unknown field":        `{"steps":1,"limits":{"type":"unlimited","legacy":true}}`,
		"trailing value":       `{"steps":1,"limits":{"type":"unlimited"}} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			var row runAccountingRow
			if err := decodeInterruptJSON(encoded, &row); err == nil {
				_, _, err = row.values()
				if err == nil {
					t.Fatalf("stored accounting %s was accepted", encoded)
				}
			}
		})
	}
}
