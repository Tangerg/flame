package protocol

import "time"

type SessionUsageRequest struct {
	SessionID string `json:"sessionId"`
}

type UsageCoverage struct {
	Calls          int `json:"calls"`
	ReportedCalls  int `json:"reportedCalls"`
	PricedCalls    int `json:"pricedCalls"`
	StartedCalls   int `json:"startedCalls"`
	CompletedCalls int `json:"completedCalls"`
	FailedCalls    int `json:"failedCalls"`
	UnknownCalls   int `json:"unknownCalls"`
}

type RecordedUsage struct {
	Total           ModelUsage    `json:"total"`
	RecordedCostUSD *float64      `json:"recordedCostUsd,omitzero"`
	Coverage        UsageCoverage `json:"coverage"`
}

type SessionUsage struct {
	RecordedUsage
	ByModel []UsageBucket `json:"byModel"`
}

type UsageSummaryRequest struct {
	SinceDays *int `json:"sinceDays,omitzero"`
}

type UsageBucket struct {
	Key string `json:"key"`
	RecordedUsage
}

type UsageSummary struct {
	RecordedUsage
	ObservedAt time.Time     `json:"observedAt"`
	Since      *time.Time    `json:"since,omitzero"`
	ByProvider []UsageBucket `json:"byProvider"`
	ByModel    []UsageBucket `json:"byModel"`
	ByDay      []UsageBucket `json:"byDay"`
	ByOrigin   []UsageBucket `json:"byOrigin"`
	ByScope    []UsageBucket `json:"byScope"`
	Sessions   int           `json:"sessions"`
	Runs       int           `json:"runs"`
}
