package execution

import (
	"strings"
	"testing"

	agentmemory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
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
