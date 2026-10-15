package plugin

import (
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestToolProposalCannotAcquireAnotherCallOrToolAuthority(t *testing.T) {
	original := chat.ToolCall{ID: "logical-call", Name: "write_record", Arguments: `{"revision":9007199254740993}`}
	for _, field := range []string{"arguments", "identity", "name", "missing identity"} {
		t.Run(field, func(t *testing.T) {
			changed := original
			switch field {
			case "arguments":
				changed.Arguments = `{"revision":9007199254740994}`
			case "identity":
				changed.ID = "another-call"
			case "name":
				changed.Name = "another_tool"
			case "missing identity":
				changed.ID = ""
			}
			if err := ValidateToolProposal(original, changed); (err == nil) != (field == "arguments") {
				t.Fatalf("transformation authority %s: %v", field, err)
			}
		})
	}
}
