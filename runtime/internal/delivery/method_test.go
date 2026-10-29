package delivery

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestRegistryViewsAreSnapshots(t *testing.T) {
	t.Parallel()

	var local MethodMeta
	for _, meta := range Contract().Metas() {
		if meta.Name == "sessions.update" {
			local = meta
			break
		}
	}
	if local.Name == "" || len(local.Errors) == 0 || len(local.CapabilityRules) == 0 {
		t.Fatal("sessions.update is missing mutable-slice metadata")
	}
	local.Errors[0] = protocol.ErrRunNotFound.Error()
	local.CapabilityRules[0].When[0].Field = "corrupted"
	local.CapabilityRules[0].Requires[0] = protocol.FeatureSkills
	got, ok := Contract().Lookup("sessions.update")
	if !ok || slices.Equal(got.Errors, local.Errors) ||
		got.CapabilityRules[0].When[0].Field == "corrupted" ||
		slices.Equal(got.CapabilityRules[0].Requires, []string{protocol.FeatureSkills}) {
		t.Fatalf("catalog exposed mutable storage: %+v", got)
	}

	snapshot, ok := Contract().Lookup("sessions.snapshot")
	if !ok || len(snapshot.Materializes) == 0 {
		t.Fatal("sessions.snapshot is missing materialized-query metadata")
	}
	snapshot.Materializes[0] = "corrupted.query"
	fresh, _ := Contract().Lookup("sessions.snapshot")
	if fresh.Materializes[0] == "corrupted.query" {
		t.Fatalf("catalog exposed mutable Materializes storage: %+v", fresh.Materializes)
	}
}

func TestMetadataEnumsRejectUnknownValuesWithoutMasqueradingAsDefaults(t *testing.T) {
	t.Parallel()

	registered, ok := Contract().Lookup("runs.list")
	if !ok {
		t.Fatal("runs.list is not registered")
	}

	tests := []struct {
		name   string
		mutate func(*MethodMeta)
		want   []string
	}{
		{name: "method name", mutate: func(meta *MethodMeta) { meta.Name = "runs" }, want: []string{`"runs"`, "dot-separated non-empty segments"}},
		{name: "method kind", mutate: func(meta *MethodMeta) { meta.Kind = MethodKind("invalid") }, want: []string{"runs.list", `MethodKind("invalid")`}},
		{name: "operation kind", mutate: func(meta *MethodMeta) { meta.Operation = OperationKind("invalid") }, want: []string{"runs.list", `OperationKind("invalid")`}},
		{name: "idempotency policy", mutate: func(meta *MethodMeta) { meta.Idempotency = IdempotencyPolicy("invalid") }, want: []string{"runs.list", `IdempotencyPolicy("invalid")`}},
		{name: "replay cursor policy", mutate: func(meta *MethodMeta) { meta.ReplayCursor = ReplayCursorPolicy("invalid") }, want: []string{"runs.list", `ReplayCursorPolicy("invalid")`}},
		{name: "query run replay cursor", mutate: func(meta *MethodMeta) { meta.ReplayCursor = ReplayCursorRun }, want: []string{"runs.list", "only a streaming method"}},
		{name: "pagination kind", mutate: func(meta *MethodMeta) { meta.Pagination = PaginationKind("invalid") }, want: []string{"runs.list", `PaginationKind("invalid")`}},
		{name: "pagination disagrees with shapes", mutate: func(meta *MethodMeta) { meta.Pagination = paginationNone }, want: []string{"runs.list", "shapes derive cursor"}},
		{name: "condition operator", mutate: func(meta *MethodMeta) { meta.CapabilityRules[0].When[0].Operator = ConditionOperator("invalid") }, want: []string{"runs.list", "includeDescendants", `ConditionOperator("invalid")`}},
		{name: "materializes itself", mutate: func(meta *MethodMeta) { meta.Materializes = []Name{meta.Name} }, want: []string{"runs.list", "cannot materialize itself"}},
		{name: "repeated materialized query", mutate: func(meta *MethodMeta) { meta.Materializes = []Name{ItemsList, ItemsList} }, want: []string{"runs.list", "items.list", "declared twice"}},
		{name: "command materialization", mutate: func(meta *MethodMeta) {
			meta.Materializes = []Name{ItemsList}
			meta.Operation = OperationCommand
		}, want: []string{"runs.list", "only a query"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			meta := registered
			test.mutate(&meta)
			err := meta.Validate()
			if err == nil {
				t.Fatal("validate accepted an unknown metadata enum")
			}
			for _, fragment := range test.want {
				if !strings.Contains(err.Error(), fragment) {
					t.Fatalf("error = %q, want fragment %q", err, fragment)
				}
			}
		})
	}

	if got := MethodKind("invalid").String(); got == kindUnary.String() || got == KindStream.String() {
		t.Fatalf("unknown method kind masquerades as %q", got)
	}
	if got := OperationKind("invalid").String(); got == OperationQuery.String() {
		t.Fatalf("unknown operation kind masquerades as %q", got)
	}
	if got := IdempotencyPolicy("invalid").String(); got == idempotencyNone.String() {
		t.Fatalf("unknown idempotency policy masquerades as %q", got)
	}
	if got := ReplayCursorPolicy("invalid").String(); got == replayCursorNone.String() {
		t.Fatalf("unknown replay cursor policy masquerades as %q", got)
	}
	if got := PaginationKind("invalid").String(); got == paginationNone.String() {
		t.Fatalf("unknown pagination kind masquerades as %q", got)
	}
	if got := ConditionOperator("invalid").String(); got == OperatorPresent.String() {
		t.Fatalf("unknown condition operator masquerades as %q", got)
	}
}

func TestRunReplayCursorRequiresRunEventFrames(t *testing.T) {
	t.Parallel()

	meta, ok := Contract().Lookup("runtime.subscribe")
	if !ok {
		t.Fatal("runtime.subscribe is not registered")
	}
	meta.ReplayCursor = ReplayCursorRun
	if err := meta.Validate(); err == nil || !strings.Contains(err.Error(), "requires RunEvent stream frames") {
		t.Fatalf("Validate error = %v, want RunEvent stream refusal", err)
	}
}

func TestPaginationIsDerivedFromWireShapes(t *testing.T) {
	t.Parallel()

	wantCursor := []Name{InterruptsList, ItemsList, modelInvocationsEndpoints, modelInvocationsList, RunsList, SessionsList, SessionsTrajectory, workspaceFilesList}
	var gotCursor []Name
	for _, method := range Contract().Metas() {
		if method.Pagination == paginationCursor {
			gotCursor = append(gotCursor, method.Name)
		}
	}
	slices.Sort(gotCursor)
	if !slices.Equal(gotCursor, wantCursor) {
		t.Fatalf("cursor-paginated methods = %v, want %v", gotCursor, wantCursor)
	}

	for _, name := range []Name{modelsList, providersList, RunsGet, toolsList, workspacesList} {
		method, ok := Contract().Lookup(name)
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		if method.Pagination != paginationNone {
			t.Errorf("%s pagination = %s, want %s", name, method.Pagination, paginationNone)
		}
	}
}

type recursiveParameter struct {
	Next *recursiveParameter `json:"next,omitempty"`
}

type AnonymousParameterScalar string

type anonymousParameterDeclaration struct {
	AnonymousParameterScalar
}

func TestMethodAdmissionRejectsUndecodableParameterDeclarations(t *testing.T) {
	registered, found := Contract().Lookup(providersList)
	if !found {
		t.Fatal("providers.list is not registered")
	}
	// Malformed declarations remain runtime fixtures so vet still checks all real DTOs.
	duplicate := reflect.StructOf([]reflect.StructField{
		{Name: "First", Type: reflect.TypeFor[string](), Tag: `json:"id"`},
		{Name: "Second", Type: reflect.TypeFor[string](), Tag: `json:"id"`},
	})
	nested := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: duplicate, Tag: `json:"value"`}})
	for _, test := range []struct {
		name  string
		shape reflect.Type
	}{
		{name: "duplicate names", shape: duplicate},
		{name: "anonymous scalar", shape: reflect.TypeFor[anonymousParameterDeclaration]()},
		{name: "invalid nested shape", shape: nested},
		{name: "scalar request", shape: reflect.TypeFor[string]()},
	} {
		t.Run(test.name, func(t *testing.T) {
			meta := registered
			meta.Params = test.shape
			if err := meta.Validate(); err == nil {
				t.Fatal("an undecodable request declaration was admitted")
			}
		})
	}
	registered.Params = reflect.TypeFor[recursiveParameter]()
	if err := registered.Validate(); err != nil {
		t.Fatalf("a valid recursive declaration was refused: %v", err)
	}
}

func TestExtensionPreparationUsesTheRegisteredMetadataOwner(t *testing.T) {
	registry := newRegistry()
	meta := MethodMeta{
		Name: "fixture.extension", GoName: "FixtureExtension",
		CapabilityRules: requires(protocol.FeaturePlugins),
		Errors:          []string{protocol.ErrRunNotFound.Error()},
	}
	registry.extension(meta,
		func(struct{}) (OperationKind, error) { return OperationQuery, nil },
		func(struct{}, context.Context, struct{}) (OperationKind, func(context.Context) (struct{}, error), error) {
			return OperationQuery, func(context.Context) (struct{}, error) { return struct{}{}, nil }, nil
		},
	)
	meta.CapabilityRules[0].Requires[0] = protocol.FeatureSkills
	meta.Errors[0] = protocol.ErrInvalidParams.Error()
	registered, found := registry.lookup(meta.Name)
	if !found {
		t.Fatal("extension was not registered")
	}
	for range 2 {
		resolved, err := registered.prepare(struct{}{}, t.Context(), struct{}{})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(resolved.Meta.Features(), []string{protocol.FeaturePlugins}) || !slices.Equal(resolved.Meta.Errors, []string{protocol.ErrRunNotFound.Error()}) {
			t.Fatalf("prepared method escaped registered ownership: %+v", resolved.Meta)
		}
		resolved.Meta.CapabilityRules[0].Requires[0] = protocol.FeatureSkills
	}
	if _, err := registered.prepare(struct{}{}, t.Context(), "wrong request type"); err == nil {
		t.Fatal("extension accepted parameters outside its registered type")
	}
}
