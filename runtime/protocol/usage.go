package protocol

type SessionUsageRequest struct {
	SessionID string `json:"sessionId"`
}

// Coverage counts retained physical attempts; missing usage or price is not zero
// consumption. Completed and failed attempts may both contain known usage.
type UsageCoverage struct {
	Calls          int `json:"calls"`
	ReportedCalls  int `json:"reportedCalls"`
	PricedCalls    int `json:"pricedCalls"`
	StartedCalls   int `json:"startedCalls"`
	CompletedCalls int `json:"completedCalls"`
	FailedCalls    int `json:"failedCalls"`
	UnknownCalls   int `json:"unknownCalls"`
}

// RecordedUsage contains known token totals and their coverage. Total.costUsd is
// exact only when every retained attempt is priced. RecordedCostUsd is the priced
// subtotal and may omit consumption by unpriced or unknown attempts.
type RecordedUsage struct {
	Total           ModelUsage    `json:"total"`
	RecordedCostUSD *float64      `json:"recordedCostUsd,omitzero"`
	Coverage        UsageCoverage `json:"coverage"`
}

// SessionUsage includes retained Session-associated physical endpoints, including
// auxiliary calls without a Run. Run metrics remain the execution projection.
type SessionUsage struct {
	RecordedUsage
	ByModel []UsageBucket `json:"byModel"`
}

type UsageSummaryRequest struct {
	// SinceDays selects attempts admitted within the preceding N calendar days.
	SinceDays *int `json:"sinceDays,omitzero"`
}

type UsageBucket struct {
	Key string `json:"key"`
	RecordedUsage
}

// UsageSummary folds every retained physical endpoint once, including active,
// Session-only and unattached calls. Run totals are not added to these sources.
type UsageSummary struct {
	RecordedUsage
	ByProvider []UsageBucket `json:"byProvider"`
	ByModel    []UsageBucket `json:"byModel"`
	ByDay      []UsageBucket `json:"byDay"`
	ByOrigin   []UsageBucket `json:"byOrigin"`
	ByScope    []UsageBucket `json:"byScope"`
	// Counts identify distinct owners represented by the selected attempts. They
	// include active Runs and do not establish task completion or billing status.
	Sessions int `json:"sessions"`
	Runs     int `json:"runs"`
}
