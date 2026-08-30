package runtimeembedded

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/embedded"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRequireCompletePageRejectsUnconsumableResults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		page *protocol.Page[string]
		want string
	}{
		{name: "nil page", want: "nil page"},
		{name: "continuation", page: protocol.NewPageWithCursor([]string{"first"}, "next"), want: "continuation cursor"},
		{name: "complete", page: protocol.NewPage([]string{"first"})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			values, err := requireCompletePage("list values", test.page)
			if test.want == "" {
				if err != nil || len(values) != 1 || values[0] != "first" {
					t.Fatalf("requireCompletePage = (%v, %v)", values, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("requireCompletePage error = %v, want %q", err, test.want)
			}
			requireRuntimeContractViolation(t, err)
		})
	}
}

type testProjection struct {
	identity string
	valid    bool
}

func (p testProjection) Validate() error {
	if !p.valid {
		return errors.New("invalid projection")
	}
	return nil
}

func TestProjectUniqueValuesOwnsCatalogValidationAndIdentity(t *testing.T) {
	t.Parallel()

	project := func(value string) testProjection {
		return testProjection{identity: value, valid: value != "invalid"}
	}
	identity := func(value testProjection) string { return value.identity }

	projected, err := projectUniqueValues("list values", []string{"first", "second"}, project, identity)
	if err != nil || len(projected) != 2 || projected[1].identity != "second" {
		t.Fatalf("projectUniqueValues = (%+v, %v)", projected, err)
	}
	for _, test := range []struct {
		name   string
		values []string
		want   string
	}{
		{name: "invalid row", values: []string{"first", "invalid"}, want: "list values item 2 is invalid"},
		{name: "duplicate identity", values: []string{"first", "first"}, want: `list values repeats "first"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := projectUniqueValues("list values", test.values, project, identity)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("projectUniqueValues error = %v, want %q", err, test.want)
			}
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestCursorTraversalRejectsDirectAndMultiStepCycles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		initial  string
		sequence []string
		wantMore []bool
		wantErr  bool
	}{
		{name: "complete", sequence: []string{"next", ""}, wantMore: []bool{true, false}},
		{name: "direct cycle", initial: "current", sequence: []string{"current"}, wantErr: true},
		{name: "multi-step cycle", sequence: []string{"first", "second", "first"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			traversal, err := newCursorTraversal("list values", test.initial, 4)
			if err != nil {
				t.Fatal(err)
			}
			for index, next := range test.sequence {
				more, err := traversal.Advance(next)
				if err != nil {
					if !test.wantErr || !strings.Contains(err.Error(), "cyclic continuation cursor") {
						t.Fatalf("Advance(%q) error = %v", next, err)
					}
					requireRuntimeContractViolation(t, err)
					return
				}
				if index < len(test.wantMore) && more != test.wantMore[index] {
					t.Fatalf("Advance(%q) more = %t, want %t", next, more, test.wantMore[index])
				}
			}
			if test.wantErr {
				t.Fatal("cursor cycle was accepted")
			}
		})
	}
}

func TestCursorTraversalRejectsInfiniteUniqueChainsAtItsOwnedCapacity(t *testing.T) {
	t.Parallel()
	traversal, err := newCursorTraversal("list values", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if more, err := traversal.Advance("first"); err != nil || !more {
		t.Fatalf("first Advance = (%t, %v), want (true, nil)", more, err)
	}
	more, err := traversal.Advance("second")
	if err == nil || more || !strings.Contains(err.Error(), "2-page traversal capacity") {
		t.Fatalf("second Advance = (%t, %v), want capacity violation", more, err)
	}
	requireRuntimeContractViolation(t, err)
	if traversal.Current() != "first" {
		t.Fatalf("cursor after rejected continuation = %q, want first", traversal.Current())
	}
}

func TestCursorTraversalBoundsRequestAndResponseCursorBytes(t *testing.T) {
	t.Parallel()
	oversized := strings.Repeat("x", maximumPaginationCursorBytes+1)
	if _, err := newCursorTraversal("list values", oversized, 2); err == nil ||
		!strings.Contains(err.Error(), "transport limit") {
		t.Fatalf("oversized initial cursor error = %v", err)
	}
	traversal, err := newCursorTraversal("list values", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	more, err := traversal.Advance(oversized)
	if err == nil || more || !strings.Contains(err.Error(), "continuation cursor larger") {
		t.Fatalf("oversized continuation = (%t, %v), want contract violation", more, err)
	}
	requireRuntimeContractViolation(t, err)
}

func TestCursorTraversalRejectsInvalidCapacityWithoutCreatingState(t *testing.T) {
	t.Parallel()
	for _, capacity := range []int{-1, 0} {
		if traversal, err := newCursorTraversal("list values", "", capacity); err == nil || traversal != nil {
			t.Fatalf("newCursorTraversal capacity %d = (%v, %v), want nil/error", capacity, traversal, err)
		}
	}
}

type modelCatalogBindingStub struct {
	providers *protocol.Page[protocol.Provider]
	models    map[string]*protocol.Page[protocol.Model]
}

func (m modelCatalogBindingStub) ListProviders(context.Context, embedded.CallOptions) (*protocol.Page[protocol.Provider], error) {
	return m.providers, nil
}

func (m modelCatalogBindingStub) ListModels(_ context.Context, request protocol.ListModelsRequest, _ embedded.CallOptions) (*protocol.Page[protocol.Model], error) {
	return m.models[request.Provider], nil
}

func TestModelCatalogRejectsEveryUnconsumableContinuation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		stub modelCatalogBindingStub
		want string
	}{
		{
			name: "providers",
			stub: modelCatalogBindingStub{providers: protocol.NewPageWithCursor([]protocol.Provider{}, "next")},
			want: "list providers",
		},
		{
			name: "models",
			stub: modelCatalogBindingStub{
				providers: protocol.NewPage([]protocol.Provider{{ID: "deepseek", CredentialRequirement: protocol.ProviderAPIKeyRequired}}),
				models: map[string]*protocol.Page[protocol.Model]{
					"deepseek": protocol.NewPageWithCursor([]protocol.Model{}, "next"),
				},
			},
			want: "list models for deepseek",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := &Runtime{modelCatalog: test.stub, meta: requestMeta("test")}
			_, err := runtime.ListModels(t.Context())
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "continuation cursor") {
				t.Fatalf("ListModels error = %v, want %q continuation failure", err, test.want)
			}
		})
	}
}
