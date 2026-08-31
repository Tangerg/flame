// Usage report values describe durable Run metering presented by the CLI.
package agent

import (
	"errors"
	"fmt"
	"strings"

	cliidentity "github.com/Tangerg/flame/cli/internal/domain/identity"
)

type UsageTotals struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	CostUSD          *float64
}

func (t UsageTotals) Validate() error {
	if t.InputTokens < 0 || t.OutputTokens < 0 || t.CacheReadTokens < 0 ||
		t.CacheWriteTokens < 0 || t.ReasoningTokens < 0 {
		return errors.New("usage totals contain a negative token count")
	}
	if t.CostUSD != nil && *t.CostUSD < 0 {
		return errors.New("usage totals contain a negative cost")
	}
	return nil
}

type UsageBucket struct {
	Key    string
	Totals UsageTotals
	Runs   int
}

func (b UsageBucket) Validate() error {
	if strings.TrimSpace(b.Key) == "" {
		return errors.New("usage bucket key is empty")
	}
	if b.Runs < 0 {
		return errors.New("usage bucket run count is negative")
	}
	return b.Totals.Validate()
}

type SessionUsageReport struct {
	SessionID string
	Total     UsageTotals
	ByModel   []UsageBucket
}

func (s SessionUsageReport) Validate() error {
	if err := cliidentity.ValidateSession(s.SessionID); err != nil {
		return fmt.Errorf("session usage report: %w", err)
	}
	if err := s.Total.Validate(); err != nil {
		return fmt.Errorf("session usage report: %w", err)
	}
	return validateBuckets("session usage report", s.ByModel)
}

type UsageSummary struct {
	Period     UsageSummaryPeriod
	Total      UsageTotals
	ByProvider []UsageBucket
	ByModel    []UsageBucket
	ByDay      []UsageBucket
	Sessions   int
	Runs       int
}

func (s UsageSummary) Validate() error {
	if s.Sessions < 0 || s.Runs < 0 {
		return errors.New("usage summary contains a negative count")
	}
	if err := s.Period.Validate(); err != nil {
		return err
	}
	if err := s.Total.Validate(); err != nil {
		return fmt.Errorf("usage summary: %w", err)
	}
	for _, breakdown := range []struct {
		name    string
		buckets []UsageBucket
	}{
		{name: "provider", buckets: s.ByProvider},
		{name: "model", buckets: s.ByModel},
		{name: "day", buckets: s.ByDay},
	} {
		if err := validateBuckets("usage summary "+breakdown.name, breakdown.buckets); err != nil {
			return err
		}
	}
	return nil
}

func validateBuckets(context string, buckets []UsageBucket) error {
	seen := make(map[string]struct{}, len(buckets))
	for index, bucket := range buckets {
		if err := bucket.Validate(); err != nil {
			return fmt.Errorf("%s bucket %d: %w", context, index+1, err)
		}
		if _, duplicate := seen[bucket.Key]; duplicate {
			return fmt.Errorf("%s repeats bucket %q", context, bucket.Key)
		}
		seen[bucket.Key] = struct{}{}
	}
	return nil
}
