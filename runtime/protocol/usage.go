package protocol

// SessionUsageRequest identifies the session whose aggregate usage is read.
type SessionUsageRequest struct {
	SessionID string `json:"sessionId"`
}

// SessionUsage aggregates all retained process-owned usage in one Session.
// ByModel keys identify the provider/model pair and follow the same spend order
// as UsageSummary. A Run's Usage keeps model names local to its selected provider.
type SessionUsage struct {
	ModelUsage
	ByModel []UsageBucket `json:"byModel,omitempty"`
}

// UsageSummaryRequest is the usage.summary body.
type UsageSummaryRequest struct {
	// SinceDays limits the report to runs finished within the last N days;
	// absence means all time, while a present value is strictly positive.
	SinceDays *int `json:"sinceDays,omitzero"`
}

// UsageBucket is one grouped slice of usage — a provider id, a "provider/model"
// pair, or a day (YYYY-MM-DD) — carrying its rolled-up tokens + cost and the
// number of runs that contributed.
type UsageBucket struct {
	Key string `json:"key"`
	ModelUsage
	Runs int `json:"runs,omitzero"`
}

// UsageSummary is the cross-session spend report (usage.summary). Every bucket
// sums process-owned Run usage, so the breakdowns reconcile with Total. Provider
// attribution follows the Run's selection; model buckets use the served model
// split when it is available. Request-detached utility-model maintenance is not
// part of Run usage.
type UsageSummary struct {
	Total      ModelUsage    `json:"total"`
	ByProvider []UsageBucket `json:"byProvider,omitempty"`
	ByModel    []UsageBucket `json:"byModel,omitempty"`
	ByDay      []UsageBucket `json:"byDay,omitempty"`
	// Sessions is the number of user-facing sessions with any recorded spend;
	// Runs is the number of finished runs counted.
	Sessions int `json:"sessions,omitzero"`
	Runs     int `json:"runs,omitzero"`
}
