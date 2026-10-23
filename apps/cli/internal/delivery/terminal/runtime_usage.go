package terminal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

type usageReport struct {
	session conversation.SessionUsageReport
	summary conversation.UsageSummary
}

func (a *app) ShowUsage(argument string) error {
	if a.usage == nil {
		return errors.New("this runtime composition has no usage service")
	}
	period, err := parseUsagePeriod(argument)
	if err != nil {
		return err
	}
	sessionID := a.session.current.ID
	a.runRuntimeReaderQuery("loading runtime usage", runtimeReaderNone,
		func(ctx context.Context) (readerDocument, error) {
			session, err := a.usage.SessionUsage(ctx, sessionID)
			if err != nil {
				return readerDocument{}, err
			}
			summary, err := a.usage.Summary(ctx, period)
			if err != nil {
				return readerDocument{}, err
			}
			return usageDocument(usageReport{session: session, summary: summary})
		})
	return nil
}

func parseUsagePeriod(argument string) (conversation.UsageSummaryPeriod, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" || strings.EqualFold(argument, "all") {
		return conversation.AllTimeUsage(), nil
	}
	days, err := strconv.Atoi(argument)
	if err != nil || days <= 0 {
		return conversation.UsageSummaryPeriod{}, errors.New("usage: /usage [positive-days|all]")
	}
	return conversation.RecentUsageDays(days)
}

func usageDocument(report usageReport) (readerDocument, error) {
	window := "all time"
	days, recent, err := report.summary.Period.Days()
	if err != nil {
		return readerDocument{}, fmt.Errorf("runtime usage document: %w", err)
	}
	if recent {
		window = fmt.Sprintf("last %d days", days)
	}
	sections := []ToolSection{
		{Title: "Current session", Style: toolSectionCode, Text: usageReportText(report.session.Total, report.session.RecordedCostUSD, report.session.Coverage)},
		{Title: "Runtime total", Style: toolSectionCode, Text: usageReportText(report.summary.Total, report.summary.RecordedCostUSD, report.summary.Coverage)},
	}
	sections = appendUsageBreakdown(sections, "By provider", report.summary.ByProvider)
	sections = appendUsageBreakdown(sections, "By model", report.summary.ByModel)
	sections = appendUsageBreakdown(sections, "By day", report.summary.ByDay)
	sections = appendUsageBreakdown(sections, "By origin", report.summary.ByOrigin)
	sections = appendUsageBreakdown(sections, "By scope", report.summary.ByScope)
	return readerDocument{
		Title: "Runtime usage", Detail: fmt.Sprintf("%s · %d attempts · %d associated sessions · %d associated runs", window, report.summary.Coverage.Calls, report.summary.Sessions, report.summary.Runs),
		Sections: sections,
	}, nil
}

func appendUsageBreakdown(sections []ToolSection, title string, buckets []protocol.UsageBucket) []ToolSection {
	if len(buckets) == 0 {
		return sections
	}
	lines := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		line := bucket.Key + "  " + usageReportText(bucket.Total, bucket.RecordedCostUSD, bucket.Coverage)
		lines = append(lines, line)
	}
	return append(sections, ToolSection{Title: title, Style: toolSectionCode, Text: strings.Join(lines, "\n")})
}

func usageTotalsText(totals protocol.ModelUsage) string {
	parts := []string{
		"input " + format.Thousands(totals.InputTokens),
		"output " + format.Thousands(totals.OutputTokens),
	}
	if totals.CacheReadTokens > 0 {
		parts = append(parts, "cache read "+format.Thousands(totals.CacheReadTokens))
	}
	if totals.CacheWriteTokens > 0 {
		parts = append(parts, "cache write "+format.Thousands(totals.CacheWriteTokens))
	}
	if totals.ReasoningTokens > 0 {
		parts = append(parts, "reasoning "+format.Thousands(totals.ReasoningTokens))
	}
	if totals.CostUSD != nil {
		parts = append(parts, format.USD(*totals.CostUSD))
	} else {
		parts = append(parts, "cost unavailable")
	}
	return strings.Join(parts, "  · ")
}

func usageReportText(total protocol.ModelUsage, cost *float64, coverage protocol.UsageCoverage) string {
	text := "known " + usageTotalsText(total)
	if cost != nil {
		text += "  · recorded price subtotal " + format.USD(*cost)
	}
	return text + fmt.Sprintf("  · usage recorded %d/%d  · priced %d/%d  · started %d  · unknown %d", coverage.ReportedCalls, coverage.Calls, coverage.PricedCalls, coverage.Calls, coverage.StartedCalls, coverage.UnknownCalls)
}
