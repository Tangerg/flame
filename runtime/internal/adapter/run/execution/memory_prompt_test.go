package execution

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPinnedMemoryPromptKeepsIdentityOrderAcrossReadAndTimestampChanges(t *testing.T) {
	base := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	items := []MemoryPromptItem{
		testMemoryPromptItem(MemoryPromptItem{ID: testAgentMemoryItemID(t, '1'), Content: "- old unpinned", UpdatedAt: base}),
		testMemoryPromptItem(MemoryPromptItem{ID: testAgentMemoryItemID(t, '2'), Content: "- pinned note", Pinned: true, UpdatedAt: base.Add(-time.Hour)}),
		testMemoryPromptItem(MemoryPromptItem{ID: testAgentMemoryItemID(t, '3'), Content: "- another pinned note", Pinned: true, UpdatedAt: base.Add(time.Hour)}),
	}
	got := strings.Split(newPinnedMemoryPrompt(items, 0).text, "\n")
	want := []string{"- pinned note", "- another pinned note"}
	if !slices.Equal(got, want) {
		t.Fatalf("rendered memory = %#v, want %#v", got, want)
	}
	items[1].UpdatedAt = base.Add(2 * time.Hour)
	slices.Reverse(items)
	if reordered := strings.Split(newPinnedMemoryPrompt(items, 0).text, "\n"); !slices.Equal(reordered, want) {
		t.Fatalf("timestamp or read order changed the pinned prefix: %#v", reordered)
	}
}

func TestPinnedMemoryPromptHonorsBudget(t *testing.T) {
	items := []MemoryPromptItem{
		testMemoryPromptItem(MemoryPromptItem{ID: testAgentMemoryItemID(t, '1'), Content: "- pinned", Pinned: true}),
		testMemoryPromptItem(MemoryPromptItem{ID: testAgentMemoryItemID(t, '2'), Content: strings.Repeat("界", 40), Pinned: true}),
	}
	prompt := newPinnedMemoryPrompt(items, 5)
	if prompt.text != "- pinned" || len(prompt.sources) != 1 || prompt.sources[0].Reference != items[0].ID {
		t.Fatalf("budgeted memory = %+v, want only pinned item", prompt)
	}
	if newPinnedMemoryPrompt(nil, 10).text != "" {
		t.Fatal("empty memory must render nothing")
	}
	if got := estimateMemoryPromptTokens(strings.Repeat("界", 100)); got != 100 {
		t.Fatalf("CJK estimate = %d, want 100", got)
	}
}

func TestPinnedMemoryPromptDoesNotLetFirstItemBypassBudget(t *testing.T) {
	prompt := newPinnedMemoryPrompt([]MemoryPromptItem{testMemoryPromptItem(MemoryPromptItem{
		ID:      testAgentMemoryItemID(t, '1'),
		Content: strings.Repeat("界", 6),
		Pinned:  true,
	})}, 5)
	if prompt.text != "" || len(prompt.sources) != 0 {
		t.Fatalf("oversized first item bypassed budget: %+v", prompt)
	}
}
