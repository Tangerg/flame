package coordination

import (
	"slices"
	"testing"
)

func TestParseMemoryFactsCanonicalizesModelMarkdown(t *testing.T) {
	got, err := parseMemoryFacts("- keep this\n\n* keep this\n1. numbered\n+ use `go test`\n")
	want := []string{"keep this", "numbered", "use `go test`"}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("parsed facts = %#v, %v, want %#v", got, err, want)
	}
}

func TestParseMemoryFactsRefusesAnEntireMalformedProposal(t *testing.T) {
	for _, response := range []string{
		"Here are the facts:\n- use go test",
		"```markdown\n- use go test\n```",
		"- use go test\nI cannot extract any more facts.",
		"- use go test\n  - nested detail",
		"- use go test\nNO_FACTS",
		"- ```markdown",
		"- ",
	} {
		if facts, err := parseMemoryFacts(response); err == nil || len(facts) != 0 {
			t.Fatalf("malformed proposal %q produced facts=%v, error=%v", response, facts, err)
		}
	}
}
