package analyzer

import (
	"github.com/Tangerg/flame/plugins/codeintel/internal/lsp"
	"testing"
)

func TestProblemIdentityIgnoresPositionButRetainsMeaning(t *testing.T) {
	before := lsp.Diagnostic{Message: "undefined: foo", Severity: 1, Range: lsp.Range{Start: lsp.Position{Line: 10}}}
	shifted := before
	shifted.Range.Start.Line = 15
	if problemKey(before) != problemKey(shifted) {
		t.Fatal("line shift created a new problem")
	}
	changed := before
	changed.Message = "undefined: bar"
	if problemKey(before) == problemKey(changed) {
		t.Fatal("different problems share an identity")
	}
}
