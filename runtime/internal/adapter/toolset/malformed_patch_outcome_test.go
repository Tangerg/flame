package toolset

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

// A hunk header that overstates its old side makes the parser read past the
// hunk body and report the next @@ as a stray line. Models get these counts
// wrong, and the Host reads an unclassified Tool error as an operation whose
// durable outcome it cannot prove, which settles the whole Run tree — the root
// and every delegated child — as lost. Nothing ran here, so the model has to be
// told instead.
func TestMalformedPatchFailsTheCallNotTheRun(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools, err := buildCWDTools(root, nil, newReadTracker(), newPathLocker())
	if err != nil {
		t.Fatal(err)
	}

	// -1,5 declares five old-side lines; the body carries two.
	patch := `--- a/notes.txt
+++ b/notes.txt
@@ -1,5 +1,5 @@
 one
-two
+TWO
@@ -10,1 +10,1 @@
-ten
+TEN
`
	arguments, err := json.Marshal(map[string]string{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	_, err = callTextTool(t.Context(), tools.applyPatch, string(arguments))
	if err == nil {
		t.Fatal("an unparseable patch was accepted")
	}

	var failure *toolcontract.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("unparseable patch returned an unclassified error (%v); "+
			"the Host settles an unclassified Tool error as an operation of unknown outcome and loses the Run tree", err)
	}
	if failure.Kind() != toolcontract.FailureKindFailed {
		t.Fatalf("failure kind = %q, want %q: nothing was refused permission, the call definitely did not happen",
			failure.Kind(), toolcontract.FailureKindFailed)
	}
	told, _ := failure.Output().Text()
	if !strings.Contains(told, "oldCount") {
		t.Fatalf("the model was told %q, which does not name the counts it has to recount", told)
	}
	if body, readErr := os.ReadFile(filepath.Join(root, "notes.txt")); readErr != nil || string(body) != "one\ntwo\n" {
		t.Fatalf("file = %q (%v), want the original: a refused patch must change nothing", body, readErr)
	}
}
