package terminal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type usageReport struct {
	session agent.SessionUsageReport
	summary agent.UsageSummary
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

func parseUsagePeriod(argument string) (agent.UsageSummaryPeriod, error) {
	argument = strings.TrimSpace(argument)
	if argument == "" || strings.EqualFold(argument, "all") {
		return agent.AllTimeUsage(), nil
	}
	days, err := strconv.Atoi(argument)
	if err != nil || days <= 0 {
		return agent.UsageSummaryPeriod{}, errors.New("usage: /usage [positive-days|all]")
	}
	return agent.RecentUsageDays(days)
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
		{Title: "Current session", Style: toolSectionCode, Language: "text", Text: usageTotalsText(report.session.Total)},
		{Title: "Runtime total", Style: toolSectionCode, Language: "text", Text: usageTotalsText(report.summary.Total)},
	}
	sections = appendUsageBreakdown(sections, "By provider", report.summary.ByProvider)
	sections = appendUsageBreakdown(sections, "By model", report.summary.ByModel)
	sections = appendUsageBreakdown(sections, "By day", report.summary.ByDay)
	return readerDocument{
		Title: "Runtime usage", Detail: fmt.Sprintf("%s · %d sessions · %d runs", window, report.summary.Sessions, report.summary.Runs),
		Sections: sections,
	}, nil
}

func appendUsageBreakdown(sections []ToolSection, title string, buckets []agent.UsageBucket) []ToolSection {
	if len(buckets) == 0 {
		return sections
	}
	lines := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		line := bucket.Key + "  " + usageTotalsText(bucket.Totals)
		if bucket.Runs > 0 {
			line += fmt.Sprintf("  · %d runs", bucket.Runs)
		}
		lines = append(lines, line)
	}
	return append(sections, ToolSection{Title: title, Style: toolSectionCode, Language: "text", Text: strings.Join(lines, "\n")})
}

func usageTotalsText(totals agent.UsageTotals) string {
	parts := []string{
		"input " + formatThousands(totals.InputTokens),
		"output " + formatThousands(totals.OutputTokens),
	}
	if totals.CacheReadTokens > 0 {
		parts = append(parts, "cache read "+formatThousands(totals.CacheReadTokens))
	}
	if totals.CacheWriteTokens > 0 {
		parts = append(parts, "cache write "+formatThousands(totals.CacheWriteTokens))
	}
	if totals.ReasoningTokens > 0 {
		parts = append(parts, "reasoning "+formatThousands(totals.ReasoningTokens))
	}
	if totals.CostUSD != nil {
		parts = append(parts, "$"+strconv.FormatFloat(*totals.CostUSD, 'f', 4, 64))
	} else {
		parts = append(parts, "cost unavailable")
	}
	return strings.Join(parts, "  · ")
}
