// Usage report values describe durable Run metering presented by the CLI.
package agent

import runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"

type SessionUsageReport struct {
	SessionID string
	Total     runtimeprotocol.ModelUsage
	ByModel   []runtimeprotocol.UsageBucket
}

type UsageSummary struct {
	Period     UsageSummaryPeriod
	Total      runtimeprotocol.ModelUsage
	ByProvider []runtimeprotocol.UsageBucket
	ByModel    []runtimeprotocol.UsageBucket
	ByDay      []runtimeprotocol.UsageBucket
	Sessions   int
	Runs       int
}
