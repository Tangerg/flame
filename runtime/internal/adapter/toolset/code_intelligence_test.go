package toolset

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/codeintel"
	"github.com/Tangerg/flame/runtime/internal/keylock"
)

type testCodeIntelligence struct {
	format                                 func(codeintel.Document) (codeintel.Transformation, error)
	acquired, inspected, formatted, closed int
}

func (s *testCodeIntelligence) AcquireCodeIntelligence(context.Context) (CodeIntelligenceUse, error) {
	s.acquired++
	return s, nil
}
func (s *testCodeIntelligence) Close() error { s.closed++; return nil }
func (s *testCodeIntelligence) Inspect(input codeintel.Inspection) (codeintel.Report, error) {
	s.inspected++
	return codeintel.Report{Version: input.Document.Version}, nil
}
func (s *testCodeIntelligence) Format(document codeintel.Document) (codeintel.Transformation, error) {
	s.formatted++
	if s.format != nil {
		return s.format(document)
	}
	content := jsontext.Value(document.Content)
	if err := content.Indent(jsontext.WithIndent("  ")); err != nil {
		return codeintel.Transformation{}, err
	}
	return codeintel.Transformation{Version: document.Version, Content: new(string(content) + "\n")}, nil
}

func TestFormattingRefusesAnotherVersionAndConcurrentReplacement(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "wrong version", true: "changed file"}[replace], func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "source.json")
			original := `{"original":1}`
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			use := &testCodeIntelligence{format: func(document codeintel.Document) (codeintel.Transformation, error) {
				version := strings.Repeat("0", 64)
				if replace {
					version = document.Version
					if err := os.WriteFile(path, []byte("external change"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				return codeintel.Transformation{Version: version, Content: new("plugin result")}, nil
			}}
			if err := formatPath(t.Context(), use, mustRoot(t, root), mustLocalExecutor(t, root), "source.json"); err == nil {
				t.Fatal("stale transformation committed")
			}
			want := original
			if replace {
				want = "external change"
			}
			if content, err := os.ReadFile(path); err != nil || string(content) != want {
				t.Fatalf("source: %q %v", content, err)
			}
		})
	}
}

func TestMutationKeepsOneUseAndReportsFormatterFailureAfterSuccessfulEdit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source.json")
	if err := os.WriteFile(path, []byte(`{"value":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	source := &testCodeIntelligence{format: func(codeintel.Document) (codeintel.Transformation, error) {
		return codeintel.Transformation{}, errors.New("formatter stopped")
	}}
	tools, err := openCWDTools(root, source, newReadTracker(), keylock.NewSet())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tools.close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := callTextTool(t.Context(), tools.readSearch[0], `{"path":"source.json"}`); err != nil {
		t.Fatal(err)
	}
	output, err := callTextTool(t.Context(), tools.edit, `{"path":"source.json","old_string":"1","new_string":"2"}`)
	if err != nil || !strings.Contains(output, "formatter stopped") {
		t.Fatalf("applied edit: %q %v", output, err)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != `{"value":2}` {
		t.Fatalf("source: %q %v", content, err)
	}
	if source.acquired != 1 || source.closed != 1 || source.formatted != 1 || source.inspected != 2 {
		t.Fatalf("use lifetime: %+v", source)
	}
}
