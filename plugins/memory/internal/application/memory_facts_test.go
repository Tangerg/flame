package coordination

import (
	"errors"
	"slices"
	"testing"

	agentmemory "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

func TestDecodeMemoryFactsPreservesContentForDomainAdmission(t *testing.T) {
	got, err := decodeMemoryFacts(`[" keep this ", "keep this", "use \"go test\"", "line one\nline two"]`)
	want := []string{" keep this ", "keep this", "use \"go test\"", "line one\nline two"}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("parsed facts = %#v, %v, want %#v", got, err, want)
	}
}

func TestDecodeMemoryFactsAcceptsAnExplicitEmptyArray(t *testing.T) {
	if facts, err := decodeMemoryFacts(" [] \n"); err != nil || facts == nil || len(facts) != 0 {
		t.Fatalf("empty proposal produced facts=%v, error=%v", facts, err)
	}
}

func TestDecodeMemoryFactsRefusesAnEntireMalformedProposal(t *testing.T) {
	for _, response := range []string{
		"",
		"null",
		`{"facts":["use go test"]}`,
		`["use go test", ["nested detail"]]`,
		`["use go test", 1]`,
		`["use go test", {}]`,
		`["use go test", true]`,
		`["use go test"] []`,
		`["use go test"] trailing commentary`,
		`["use go test",]`,
		"```json\n[\"use go test\"]\n```",
		"Here are the facts:\n- use go test",
		"```markdown\n- use go test\n```",
		"- use go test\nI cannot extract any more facts.",
		"- use go test\n  - nested detail",
		"- use go test\nNO_FACTS",
		"- ```markdown",
		"- ",
	} {
		if facts, err := decodeMemoryFacts(response); !errors.Is(err, agentmemory.ErrInvalidFacts) || len(facts) != 0 {
			t.Fatalf("malformed proposal %q produced facts=%v, error=%v", response, facts, err)
		}
	}
}
