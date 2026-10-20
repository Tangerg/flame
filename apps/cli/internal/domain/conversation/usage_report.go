package conversation

import (
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

type SessionUsageReport struct {
	SessionID       string
	Total           runtimeprotocol.ModelUsage
	Coverage        runtimeprotocol.UsageCoverage
	RecordedCostUSD *float64
	ByModel         []runtimeprotocol.UsageBucket
}

type UsageSummary struct {
	Period          UsageSummaryPeriod
	Total           runtimeprotocol.ModelUsage
	Coverage        runtimeprotocol.UsageCoverage
	RecordedCostUSD *float64
	ByProvider      []runtimeprotocol.UsageBucket
	ByModel         []runtimeprotocol.UsageBucket
	ByDay           []runtimeprotocol.UsageBucket
	ByOrigin        []runtimeprotocol.UsageBucket
	ByScope         []runtimeprotocol.UsageBucket
	Sessions        int
	Runs            int
}
