package transcript

import (
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/resourceidentity"
)

func TestSearchHitOwnsExactResourceProvenance(t *testing.T) {
	valid := SearchHit{
		SessionID: "ses_1", RunID: "run_1", ItemID: "item_1",
		Kind: UserMessage, CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid hit: %v", err)
	}
	for name, mutate := range map[string]func(*SearchHit){
		"Session whitespace": func(hit *SearchHit) { hit.SessionID = "ses_ one" },
		"Run non-printing":   func(hit *SearchHit) { hit.RunID = "run_\u200bhidden" },
		"Item oversized": func(hit *SearchHit) {
			hit.ItemID = strings.Repeat("界", resourceidentity.MaximumCharacters+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("Validate accepted corrupt provenance")
			}
		})
	}
}
