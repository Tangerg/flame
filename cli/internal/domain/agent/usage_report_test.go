package agent

import (
	"testing"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

func TestUsageReportsRejectNegativeAndDuplicateValues(t *testing.T) {
	cost := 1.25
	report := SessionUsageReport{
		SessionID: "ses_1", Total: runtimeprotocol.ModelUsage{InputTokens: 10, CostUSD: &cost},
		ByModel: []runtimeprotocol.UsageBucket{{Key: "provider/model", Runs: 1}},
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	report.ByModel = append(report.ByModel, report.ByModel[0])
	if err := report.Validate(); err == nil {
		t.Fatal("duplicate model bucket was accepted")
	}
	report.ByModel = []runtimeprotocol.UsageBucket{{}}
	if err := report.Validate(); err == nil {
		t.Fatal("usage bucket without an identity was accepted")
	}
	summary := UsageSummary{Period: AllTimeUsage(), Total: runtimeprotocol.ModelUsage{InputTokens: -1}}
	if err := summary.Validate(); err == nil {
		t.Fatal("negative usage was accepted")
	}
}
