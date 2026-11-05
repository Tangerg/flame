package toolset

import (
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/keylock"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// A hunk header that overstates its old side makes the parser read past the
// hunk body and report the next @@ as a stray line. Models get these counts
// wrong, and the Host reads an unclassified Tool error as an operation whose
// durable outcome it cannot prove, which settles the whole Run tree — the root
// and every delegated child — as lost. Nothing ran here, so the model has to be
// told instead.
func TestMalformedPatchIsRejectedBeforeExecution(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tools, err := openCWDTools(root, nil, newReadTracker(), keylock.NewSet())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tools.close(); err != nil {
			t.Error(err)
		}
	})

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
	_, _, err = prepareTestInvocation(tools.applyPatch, string(arguments))
	rejection, rejected := toolcontract.ClassifyInputRejection(err)
	if !rejected {
		t.Fatalf("unparseable patch admission = %v, want model-correctable input rejection", err)
	}
	if told := rejection.Message(); !strings.Contains(told, "oldCount") {
		t.Fatalf("the model was told %q, which does not name the counts it has to recount", told)
	}
	if body, readErr := os.ReadFile(filepath.Join(root, "notes.txt")); readErr != nil || string(body) != "one\ntwo\n" {
		t.Fatalf("file = %q (%v), want the original: a refused patch must change nothing", body, readErr)
	}
}
