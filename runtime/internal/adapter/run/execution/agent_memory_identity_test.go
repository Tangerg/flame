package execution

import (
	"strings"
	"testing"

	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func testAgentMemoryItemID(t *testing.T, digit byte) agentmemory.ItemID {
	t.Helper()
	id, err := agentmemory.ParseItemID(agentmemory.ItemIDPrefix + strings.Repeat(
		string(digit),
		agentmemory.MaximumItemIDCharacters-len(agentmemory.ItemIDPrefix),
	))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testMemoryPromptItem(snapshot agentmemory.ItemSnapshot) MemoryPromptItem {
	item := testsupport.MustAgentMemoryItem(snapshot)
	return MemoryPromptItem{ID: item.ID().String(), Content: item.Content(), Pinned: item.Pinned(), UpdatedAt: item.UpdatedAt()}
}
