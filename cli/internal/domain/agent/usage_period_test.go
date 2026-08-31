package agent

import "testing"

func TestSummaryPeriodSeparatesAllTimeFromRecentDays(t *testing.T) {
	t.Parallel()

	if days, recent, err := AllTimeUsage().Days(); err != nil || recent || days != 0 {
		t.Fatalf("all-time Days = (%d, %t, %v), want absent", days, recent, err)
	}
	recent, err := RecentUsageDays(30)
	if err != nil {
		t.Fatal(err)
	}
	if days, present, err := recent.Days(); err != nil || !present || days != 30 {
		t.Fatalf("recent Days = (%d, %t, %v), want 30", days, present, err)
	}
	for _, days := range []int{0, -1} {
		if _, err := RecentUsageDays(days); err == nil {
			t.Fatalf("RecentUsageDays(%d) returned no error", days)
		}
	}
	if err := (UsageSummaryPeriod{}).Validate(); err == nil {
		t.Fatal("zero period was accepted as all-time")
	}
	if err := (UsageSummaryPeriod{kind: allTimeUsagePeriod, days: 1}).Validate(); err == nil {
		t.Fatal("all-time period carrying days was accepted")
	}
}
