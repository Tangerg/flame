package agentexec

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
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
