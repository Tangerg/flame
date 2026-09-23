package accounting

import (
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestNewTokensFoldsAbsentBreakdownsAndRejectsImpossibleReports(t *testing.T) {
	reported := chat.Usage{
		InputTokens: 10, OutputTokens: 4,
		ReasoningTokens: new(int64(0)),
	}
	tokens, err := NewTokens(reported)
	if err != nil {
		t.Fatalf("NewTokens: %v", err)
	}
	want := Tokens{InputTokens: 10, OutputTokens: 4}
	if tokens != want {
		t.Fatalf("tokens = %+v, want %+v", tokens, want)
	}

	if _, err := NewTokens(chat.Usage{InputTokens: 1, OutputTokens: 1, ReasoningTokens: new(int64(2))}); err == nil {
		t.Fatal("NewTokens accepted reasoning tokens exceeding output")
	}
}

func TestReportedUsageKeepsAbsentDistinctFromReportedZero(t *testing.T) {
	absent := chat.Usage{InputTokens: 5, OutputTokens: 1}
	zero := chat.Usage{InputTokens: 5, OutputTokens: 1, CacheReadInputTokens: new(int64(0))}
	if ReportedUsageEqual(absent, zero) {
		t.Fatal("an unsupported dimension compared equal to a reported zero")
	}
	if !ReportedUsageEqual(zero, CloneReportedUsage(zero)) {
		t.Fatal("clone changed the reported fact")
	}

	// Both fold to the same cumulative counter: the distinction survives only in
	// the reported value itself.
	absentTokens, err := NewTokens(absent)
	if err != nil {
		t.Fatalf("NewTokens(absent): %v", err)
	}
	zeroTokens, err := NewTokens(zero)
	if err != nil {
		t.Fatalf("NewTokens(zero): %v", err)
	}
	if absentTokens != zeroTokens {
		t.Fatalf("cumulative counters disagree: %+v vs %+v", absentTokens, zeroTokens)
	}
}

func TestCloneReportedUsageDetachesBreakdowns(t *testing.T) {
	reasoning := int64(3)
	source := chat.Usage{InputTokens: 9, OutputTokens: 4, ReasoningTokens: &reasoning}
	cloned := CloneReportedUsage(source)
	reasoning = 99
	if cloned.ReasoningTokens == nil || *cloned.ReasoningTokens != 3 {
		t.Fatalf("clone aliased the caller's breakdown: %+v", cloned.ReasoningTokens)
	}
}
