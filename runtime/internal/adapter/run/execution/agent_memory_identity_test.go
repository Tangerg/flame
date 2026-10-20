package execution

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"
)

func testAgentMemoryItemID(t *testing.T, digit byte) string {
	t.Helper()
	return "mem_" + strings.Repeat(string(digit), 32)
}
func testMemoryPromptItem(item MemoryPromptItem) MemoryPromptItem {
	if item.ID == "" {
		item.ID = fmt.Sprintf("mem_%x", sha256.Sum256([]byte(item.Content)))
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Unix(1, 0).UTC()
	}
	return item
}
