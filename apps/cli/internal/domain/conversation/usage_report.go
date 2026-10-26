package conversation

import "github.com/Tangerg/flame/packages/go-sdk/protocol"

type SessionUsageReport struct {
	SessionID       string
	Total           protocol.ModelUsage
	Coverage        protocol.UsageCoverage
	RecordedCostUSD *float64
	ByModel         []protocol.UsageBucket
}

type UsageSummary struct {
	Period          UsageSummaryPeriod
	Total           protocol.ModelUsage
	Coverage        protocol.UsageCoverage
	RecordedCostUSD *float64
	ByProvider      []protocol.UsageBucket
	ByModel         []protocol.UsageBucket
	ByDay           []protocol.UsageBucket
	ByOrigin        []protocol.UsageBucket
	ByScope         []protocol.UsageBucket
	Sessions        int
	Runs            int
}
