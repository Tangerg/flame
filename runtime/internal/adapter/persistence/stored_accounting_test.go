package persistence

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

func TestRunUsageDistinguishesMissingReportFromReportedZero(t *testing.T) {
	missing, err := decodeRunMetrics("", 0, 0)
	_, present := missing.Usage()
	if err != nil || present {
		t.Fatalf("missing report = %+v, %v", missing, err)
	}
	reported, err := decodeRunMetrics(`{"reported":{}}`, 1, 0)
	_, present = reported.Usage()
	if err != nil || !present {
		t.Fatalf("reported zero = %+v, %v", reported, err)
	}
}

func TestRunAccountingAndFailureRejectInvalidStoredValues(t *testing.T) {
	for _, encoded := range []string{
		`null`,
		`{"InputTokens":7}`,
		`{"inputTokens":7,"future":true}`,
		`{"reported":{"byModel":{"provider/model":{"OutputTokens":7}}}}`,
		`{"reported":{"byModel":{"provider/model":{"future":true}}}}`,
	} {
		t.Run(encoded, func(t *testing.T) {
			if _, err := decodeRunMetrics(encoded, 1, 0); err == nil {
				t.Fatal("invalid stored accounting became reported usage")
			}
		})
	}
	for _, encoded := range []string{
		`{"failure":{"kind":"internal_error","Detail":"lost diagnostic"}}`,
		`{"failure":{"kind":"internal_error","future":true}}`,
	} {
		t.Run(encoded, func(t *testing.T) {
			if _, err := decodeRunEnding(string(run.OutcomeFailed), encoded); err == nil {
				t.Fatal("unknown stored failure field was silently discarded")
			}
		})
	}
}

func TestUnreportedModelIdentitySurvivesRestart(t *testing.T) {
	var m run.Metrics
	first, err := m.AccrueModelCall("missing", nil, accounting.Cost{})
	if err != nil {
		t.Fatal(err)
	}
	row, err := runMetricsRow(first)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := decodeRunMetrics(row.usage, row.steps, 0)
	if err != nil || !restored.Equal(first) {
		t.Fatalf("missing model after reload: %v", err)
	}
	price, _ := accounting.NewCost(1)
	next, err := restored.AccrueModelCall("other", &chat.Usage{InputTokens: 1}, price)
	if err != nil {
		t.Fatal(err)
	}
	usage, _ := next.Usage()
	if _, known := usage.ByModel["missing"].Cost.USD(); known || !usage.ByModel["other"].Cost.Equal(price) {
		t.Fatalf("model prices: %+v", usage.ByModel)
	}
}
