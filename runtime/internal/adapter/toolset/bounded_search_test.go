package toolset

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/jsonschema"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type runtimeSearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type runtimeGrepResponse struct {
	Matches   []runtimeSearchMatch `json:"matches"`
	Total     int                  `json:"total"`
	Truncated bool                 `json:"truncated"`
}

type runtimeGlobResponse struct {
	Paths     []string `json:"paths"`
	Total     int      `json:"total"`
	Truncated bool     `json:"truncated"`
}

func TestRuntimeSearchDoesNotDependOnHostSearchBinaries(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "dependency"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "dependency", "ignored.go"), []byte("package ignored\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Runtime search is an in-process product capability. Host PATH contents
	// must not decide whether the model can search a workspace.
	t.Setenv("PATH", t.TempDir())

	globBody, err := callTextTool(t.Context(), namedDirectTool(t, root, "glob"), `{"pattern":"**/*.go"}`)
	if err != nil {
		t.Fatalf("glob without host find: %v", err)
	}
	var glob runtimeGlobResponse
	if unmarshalErr := json.Unmarshal([]byte(globBody), &glob); unmarshalErr != nil {
		t.Fatalf("decode glob response: %v", unmarshalErr)
	}
	if len(glob.Paths) != 1 || glob.Paths[0] != "main.go" || glob.Total != 1 || glob.Truncated {
		t.Fatalf("glob response = %+v, want one ignore-aware result", glob)
	}

	grepBody, err := callTextTool(t.Context(), namedDirectTool(t, root, "grep"), `{"pattern":"package"}`)
	if err != nil {
		t.Fatalf("grep without host rg/grep: %v", err)
	}
	var grep runtimeGrepResponse
	if err := json.Unmarshal([]byte(grepBody), &grep); err != nil {
		t.Fatalf("decode grep response: %v", err)
	}
	if len(grep.Matches) != 1 || grep.Matches[0].Path != "main.go" || grep.Total != 1 || grep.Truncated {
		t.Fatalf("grep response = %+v, want one ignore-aware result", grep)
	}
}

func TestRuntimeGrepReportsExactTotalBeyondRetainedPrefix(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("needle\n", 300)
	if err := os.WriteFile(filepath.Join(root, "many.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := callTextTool(t.Context(), namedDirectTool(t, root, "grep"), `{"pattern":"needle","max_results":2}`)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	var response runtimeGrepResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode grep response: %v", err)
	}
	if len(response.Matches) != 2 || response.Total != 300 || !response.Truncated {
		t.Fatalf("grep response = {matches:%d total:%d truncated:%t}, want 2/300/true", len(response.Matches), response.Total, response.Truncated)
	}
}

func TestRuntimeSearchAcceptsSelectedFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "mime"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mimes.go", "other.go"} {
		if err := os.WriteFile(filepath.Join(root, "mime", name), []byte("package mime\nfunc Match() {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, selected := range []string{"mime/mimes.go", filepath.Join(root, "mime", "mimes.go")} {
		t.Run(selected, func(t *testing.T) {
			args, err := json.Marshal(map[string]string{"path": selected, "pattern": "^func "})
			if err != nil {
				t.Fatal(err)
			}
			body, err := callTextTool(t.Context(), namedDirectTool(t, root, "grep"), string(args))
			if err != nil {
				t.Fatal(err)
			}
			var result runtimeGrepResponse
			if err := json.Unmarshal([]byte(body), &result); err != nil {
				t.Fatal(err)
			}
			if result.Total != 1 || len(result.Matches) != 1 || result.Matches[0].Path != "mime/mimes.go" || result.Matches[0].Line != 2 {
				t.Fatalf("selected file grep = %+v", result)
			}
			args, err = json.Marshal(map[string]string{"path": selected, "pattern": "*.go"})
			if err != nil {
				t.Fatal(err)
			}
			body, err = callTextTool(t.Context(), namedDirectTool(t, root, "glob"), string(args))
			if err != nil {
				t.Fatal(err)
			}
			var paths runtimeGlobResponse
			if err := json.Unmarshal([]byte(body), &paths); err != nil {
				t.Fatal(err)
			}
			if paths.Total != 1 || len(paths.Paths) != 1 || paths.Paths[0] != "mime/mimes.go" {
				t.Fatalf("selected file glob = %+v", paths)
			}
		})
	}
}

func TestRuntimeSearchReturnsDefiniteFailureForInvalidRequests(t *testing.T) {
	for _, test := range []struct{ name, args string }{
		{"grep", `{"pattern":"["}`},
		{"glob", `{"pattern":"["}`},
		{"grep", `{"pattern":"x","path":"missing.go"}`},
		{"glob", `{"pattern":"*.go","path":"missing.go"}`},
	} {
		t.Run(test.name+test.args, func(t *testing.T) {
			_, err := callTextTool(t.Context(), namedDirectTool(t, t.TempDir(), test.name), test.args)
			failure, known := errors.AsType[*toolcontract.Failure](err)
			if !known || failure.Kind() != toolcontract.FailureKindFailed || failure.Validate() != nil {
				t.Fatalf("search error = %v, want definite failed result", err)
			}
			if text, _ := failure.Output().Text(); text == "" {
				t.Fatal("failed search has no model feedback")
			}
		})
	}
}

func TestRuntimeGlobReportsExactTotalBeyondRetainedPrefix(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	body, err := callTextTool(t.Context(), namedDirectTool(t, root, "glob"), `{"pattern":"*.go","max_results":2}`)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	var response runtimeGlobResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode glob response: %v", err)
	}
	if len(response.Paths) != 2 || response.Total != 3 || !response.Truncated {
		t.Fatalf("glob response = {paths:%d total:%d truncated:%t}, want 2/3/true", len(response.Paths), response.Total, response.Truncated)
	}
}

func TestRuntimeGrepExcludesUnpageableTextFileAsAWhole(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pathological.txt"), []byte(strings.Repeat("needle", (1<<20)/6+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := callTextTool(t.Context(), namedDirectTool(t, root, "grep"), `{"pattern":"needle","max_results":1}`)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(body) > 64<<10 {
		t.Fatalf("grep response materialized %d bytes from an unpageable line", len(body))
	}
	var response runtimeGrepResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode grep response: %v", err)
	}
	if len(response.Matches) != 0 || response.Total != 0 || response.Truncated {
		t.Fatalf("grep response = %+v, want pathological file excluded as a whole", response)
	}
}

func TestRuntimeGrepBoundsEncodedToolResult(t *testing.T) {
	root := t.TempDir()
	line := strings.Repeat("\x01", 2<<10) + "needle\n"
	if err := os.WriteFile(filepath.Join(root, "escaped.txt"), []byte(strings.Repeat(line, 1000)), 0o600); err != nil {
		t.Fatal(err)
	}
	body, err := callTextTool(t.Context(), namedDirectTool(t, root, "grep"), `{"pattern":"needle","max_results":1000}`)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(body) > 1<<20 {
		t.Fatalf("encoded grep response = %d bytes, want at most 1 MiB", len(body))
	}
	var response runtimeGrepResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode grep response: %v", err)
	}
	if response.Total != 1000 || !response.Truncated || len(response.Matches) == 0 || len(response.Matches) >= response.Total {
		t.Fatalf("grep response = {matches:%d total:%d truncated:%t}, want bounded non-empty prefix of 1000", len(response.Matches), response.Total, response.Truncated)
	}
}

func TestRuntimeSearchContractsContainOnlyComposableInputs(t *testing.T) {
	definition := namedDirectTool(t, t.TempDir(), "grep").Definition()
	schema := string(definition.InputSchema)
	for _, retired := range []string{
		`"file_glob"`, `"file_type"`, `"ignore_case"`, `"multiline"`,
		`"before_context_lines"`, `"after_context_lines"`, `"output_mode"`,
	} {
		if strings.Contains(schema, retired) {
			t.Errorf("grep schema retains process-specific or overlapping field %s: %s", retired, schema)
		}
	}
	for _, required := range []string{`"pattern"`, `"path"`, `"max_results"`} {
		if !strings.Contains(schema, required) {
			t.Errorf("grep schema is missing %s: %s", required, schema)
		}
	}
	globSchema := string(namedDirectTool(t, t.TempDir(), "glob").Definition().InputSchema)
	if strings.Contains(globSchema, `"ignore_case"`) {
		t.Errorf("glob schema retains overlapping field ignore_case: %s", globSchema)
	}
	for _, required := range []string{`"pattern"`, `"path"`, `"max_results"`} {
		if !strings.Contains(globSchema, required) {
			t.Errorf("glob schema is missing %s: %s", required, globSchema)
		}
	}
}

func TestRuntimeSearchSchemasAdvertiseTheirOwnersLimits(t *testing.T) {
	for _, test := range []struct {
		name          string
		patternLength int
	}{
		{"grep", maxRuntimeSearchPatternBytes}, {"glob", maxRuntimeGlobPatternBytes},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema, err := jsonschema.Parse(namedDirectTool(t, t.TempDir(), test.name).Definition().InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range []struct {
				name  string
				limit int
			}{
				{"pattern", test.patternLength}, {"path", maxRuntimeSearchPathBytes},
			} {
				for _, length := range []int{field.limit, field.limit + 1} {
					input := map[string]any{"pattern": "x"}
					input[field.name] = strings.Repeat("x", length)
					raw, err := json.Marshal(input)
					if err != nil {
						t.Fatal(err)
					}
					err = schema.Validate(raw)
					if length > field.limit {
						if !errors.Is(err, jsonschema.ErrValidation) {
							t.Fatalf("excess %s length admitted: %v", field.name, err)
						}
					} else if err != nil {
						t.Fatalf("valid %s length refused: %v", field.name, err)
					}
				}
			}
		})
	}
}

func TestRuntimeSearchRejectsNumericDefaultSentinels(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, name := range []string{"grep", "glob"} {
		if _, err := callTextTool(t.Context(), namedDirectTool(t, root, name), `{"pattern":"x","max_results":0}`); err == nil {
			t.Errorf("%s accepted max_results=0 instead of requiring omission", name)
		}
	}
}

func TestRuntimeSearchSchemaAndExecutionUseExplicitTextUnits(t *testing.T) {
	for _, test := range []struct {
		name         string
		patternBytes int
	}{
		{"grep", maxRuntimeSearchPatternBytes}, {"glob", maxRuntimeGlobPatternBytes},
	} {
		t.Run(test.name, func(t *testing.T) {
			tools := newRuntimeSearchTools("")
			executable := tools.grep
			if test.name == "glob" {
				executable = tools.glob
			}
			schema, err := jsonschema.Parse(executable.Definition().InputSchema)
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range []struct {
				name    string
				budget  int
				failure string
			}{
				{"pattern", test.patternBytes, "invalid " + test.name + " pattern"},
				{"path", maxRuntimeSearchPathBytes, "invalid search path"},
			} {
				input := map[string]any{"pattern": "x"}
				input[field.name] = strings.Repeat("界", field.budget/len("界")+1)
				raw, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				if err := schema.Validate(raw); err != nil {
					t.Fatalf("Unicode characters within schema bound refused: %v", err)
				}
				if _, err := callTextTool(t.Context(), executable, string(raw)); err == nil || !strings.Contains(err.Error(), field.failure) {
					t.Fatalf("%s must refuse UTF-8 bytes before resolving the empty root: %v", field.name, err)
				}
			}
			for _, limit := range []int{1, workspaceapp.MaxGrepLimit, workspaceapp.MaxGrepLimit + 1} {
				raw, err := json.Marshal(runtimeGrepRequest{Pattern: "x", MaxResults: &limit})
				if err != nil {
					t.Fatal(err)
				}
				err = schema.Validate(raw)
				if limit > workspaceapp.MaxGrepLimit {
					if !errors.Is(err, jsonschema.ErrValidation) {
						t.Fatalf("excess result limit admitted: %v", err)
					}
				} else if err != nil {
					t.Fatalf("valid result limit %d refused: %v", limit, err)
				}
			}
		})
	}
}

func TestRuntimeSearchRejectsUndecodableIntegersAtAdmission(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"grep", "glob"} {
		binding, err := toolcontract.Bind(namedDirectTool(t, root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, arguments := range []string{
			`{"pattern":"x","max_results":1e1}`,
			`{"pattern":"x","max_results":10.0}`,
		} {
			if _, err := binding.Contract().Prepare(chat.ToolCall{ID: "search", Name: name, Arguments: arguments}, nil); !errors.Is(err, toolcontract.ErrInvalidInvocation) {
				t.Errorf("%s admitted %s: %v", name, arguments, err)
			}
		}
		if _, err := binding.Contract().Prepare(chat.ToolCall{ID: "search", Name: name, Arguments: `{"pattern":"x","max_results":10}`}, nil); err != nil {
			t.Fatalf("%s rejected valid integer: %v", name, err)
		}
	}
}

func TestRuntimeSearchConfinesPathsAndPreservesCancellation(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	for _, name := range []string{"glob", "grep"} {
		t.Run(name+" lexical escape", func(t *testing.T) {
			arguments := `{"pattern":"*","path":"../"}`
			if name == "grep" {
				arguments = `{"pattern":"secret","path":"../"}`
			}
			_, err := callTextTool(t.Context(), namedDirectTool(t, root, name), arguments)
			failure, known := errors.AsType[*toolcontract.Failure](err)
			if !known || !errors.Is(failure.Cause(), workspaceapp.ErrPathOutsideRoot) {
				t.Fatalf("%s error = %v, want ErrPathOutsideRoot", name, err)
			}
		})
		t.Run(name+" symlink escape", func(t *testing.T) {
			arguments := `{"pattern":"*","path":"outside"}`
			if name == "grep" {
				arguments = `{"pattern":"secret","path":"outside"}`
			}
			_, err := callTextTool(t.Context(), namedDirectTool(t, root, name), arguments)
			failure, known := errors.AsType[*toolcontract.Failure](err)
			if !known || !errors.Is(failure.Cause(), workspaceapp.ErrPathOutsideRoot) {
				t.Fatalf("%s error = %v, want ErrPathOutsideRoot", name, err)
			}
		})
		t.Run(name+" canceled", func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err := callTextTool(ctx, namedDirectTool(t, root, name), `{"pattern":"*"}`)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s error = %v, want context.Canceled", name, err)
			}
		})
		t.Run(name+" cancellation cause", func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			cause := errors.New("execution owner stopped")
			cancel(cause)
			_, err := callTextTool(ctx, namedDirectTool(t, root, name), `{"pattern":"x"}`)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("search cancellation = %v, want context.Canceled", err)
			}
			if context.Cause(ctx) != cause {
				t.Fatalf("execution cancellation cause = %v, want original cause", context.Cause(ctx))
			}
			if !errors.Is(err, cause) {
				t.Fatalf("search cancellation discarded the owner cause: %v", err)
			}
			if _, known := errors.AsType[*toolcontract.Failure](err); known {
				t.Fatal("cancellation became model feedback")
			}
		})
	}
}

func namedDirectTool(t *testing.T, root, name string) toolcontract.Tool {
	t.Helper()
	for _, candidate := range mustDirectTools(t, root) {
		if candidate.Definition().Name == name {
			return candidate
		}
	}
	t.Fatalf("direct tool %q is not registered", name)
	return nil
}

func TestRuntimeSearchPreservesCancellationAtItsBoundary(t *testing.T) {
	for _, name := range []string{"glob", "grep"} {
		t.Run(name, func(t *testing.T) {
			cause := errors.New("search request retired")
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(cause)
			var err error
			if name == "glob" {
				_, err = runtimeGlob(ctx, t.TempDir(), runtimeGlobRequest{Pattern: "*"})
			} else {
				_, err = runtimeGrep(ctx, t.TempDir(), runtimeGrepRequest{Pattern: "x"})
			}
			if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Fatalf("search error = %v, want cancellation category and owner cause", err)
			}
		})
	}
}
