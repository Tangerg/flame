package arch

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

// TestGeneratedContractHasNoDrift compares freshly generated artifacts with
// their published copies.
//
// It is the only mechanism that notices when the code and the published contract
// stop agreeing. Every other check in this package guards a structural rule; this
// one guards a FACT — that the method surface, capability policy, error registry
// and shape specs a client reads are the ones the dispatcher actually implements.
// Without it, "generated" degrades into "generated once".
func TestGeneratedContractHasNoDrift(t *testing.T) {
	root := moduleRoot(t)
	regenerated, _, regeneratedValidators := regenerateContract(t, root)

	// Some artifacts land outside contract/'s root — the published TypeScript binding
	// in its own subdirectory and the Go validator beside the shapes it checks. Their
	// homes may hold other files, so the rule runs one way: every file the generator
	// writes there must match the copy in the tree.
	for _, outside := range []struct{ fresh, committed string }{
		{regeneratedValidators, filepath.Join(dependencyModuleRoot(t, sdkModulePath), validatorDir)},
	} {
		for _, name := range artifactNames(t, outside.fresh) {
			if !bytes.Equal(readArtifact(t, outside.fresh, name), readArtifact(t, outside.committed, name)) {
				t.Errorf("%s is stale — run `go generate ./...` and commit the result", name)
			}
		}
	}

	fresh := artifactNames(t, regenerated)
	committed := slices.DeleteFunc(artifactNames(t, filepath.Join(root, "contract")), func(name string) bool {
		return name == "go-api.json" // Authored and checked by tooling/goapi.
	})
	for _, name := range fresh {
		if !slices.Contains(committed, name) {
			t.Errorf("the generator writes %s and the tree has no copy — run `go generate ./...` and commit it", name)
		}
	}
	for _, name := range committed {
		if !slices.Contains(fresh, name) {
			t.Errorf("contract/%s is no longer generated — delete it rather than leaving an artifact nothing authors", name)
		}
	}
	for _, name := range fresh {
		if !slices.Contains(committed, name) {
			continue
		}
		if !bytes.Equal(readArtifact(t, regenerated, name), readArtifact(t, filepath.Join(root, "contract"), name)) {
			t.Errorf("contract/%s is stale — run `go generate ./...` and commit the result", name)
		}
	}
}

// Where the runtime publishes its generated TypeScript binding, mirroring the
// generation directive on the dispatch contract catalog. Client modules own how
// they consume or vendor this output; the runtime never writes into their trees.
const (
	tsWireDir     = "contract/samples"
	validatorDir  = "protocol"
	validatorFile = "wire_constraints.generated.go"
)

func regenerateContract(t *testing.T, root string) (artifacts, typescript, validators string) {
	t.Helper()

	artifacts, typescript, validators = t.TempDir(), t.TempDir(), t.TempDir()
	cmd := exec.Command("go", "run", "github.com/Tangerg/flame/runtime/cmd/contractgen",
		"-out", artifacts, "-ts", typescript, "-validators", validators, "-samples", filepath.Join(root, "contract", "samples"),
		"-go-local", t.TempDir(), "-go-client", t.TempDir())
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run contractgen: %v\n%s", err, combined)
	}
	return artifacts, typescript, validators
}

func artifactNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	slices.Sort(out)
	return out
}

func readArtifact(t *testing.T, dir, name string) []byte {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(dir, name), err)
	}
	return content
}

// TestGeneratedContractIsSubstantive stops the drift gate from passing
// vacuously: an empty manifest would compare equal to an empty manifest forever.
func TestGeneratedContractIsSubstantive(t *testing.T) {
	root := moduleRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "contract", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		ProtocolVersion  string     `json:"protocolVersion"`
		Methods          []struct{} `json:"methods"`
		StreamingMethods []string   `json:"streamingMethods"`
		HTTPEndpoints    []struct{} `json:"httpEndpoints"`
		Errors           struct {
			Types []struct{} `json:"types"`
		} `json:"errors"`
		CapabilityPolicy    []struct{} `json:"capabilityPolicy"`
		RunEventPolicy      []struct{} `json:"runEventPolicy"`
		CarriedShapes       []struct{} `json:"carriedShapes"`
		ResultPresentations []struct{} `json:"toolResultPresentations"`
		Unions              []struct{} `json:"unions"`
		Constraints         []struct{} `json:"objectConstraints"`
		ValueConstraints    []struct{} `json:"valueConstraints"`
		SystemInvariants    []struct{} `json:"systemInvariants"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	sections := map[string]int{
		"protocolVersion":         len(manifest.ProtocolVersion),
		"methods":                 len(manifest.Methods),
		"streamingMethods":        len(manifest.StreamingMethods),
		"httpEndpoints":           len(manifest.HTTPEndpoints),
		"errors.types":            len(manifest.Errors.Types),
		"capabilityPolicy":        len(manifest.CapabilityPolicy),
		"carriedShapes":           len(manifest.CarriedShapes),
		"toolResultPresentations": len(manifest.ResultPresentations),
		"runEventPolicy":          len(manifest.RunEventPolicy),
		"unions":                  len(manifest.Unions),
		"objectConstraints":       len(manifest.Constraints),
		"valueConstraints":        len(manifest.ValueConstraints),
		"systemInvariants":        len(manifest.SystemInvariants),
	}
	for section, count := range sections {
		if count == 0 {
			t.Errorf("manifest section %q is empty; the drift gate would pass on nothing", section)
		}
	}
}

// TestRegistryShapeRulesReachTheGoValidator closes the server-side half of the
// three-way contract: every union, conditional object and value-constrained shape
// registered for schema/TypeScript also owns a generated validateWire method.
func TestRegistryShapeRulesReachTheGoValidator(t *testing.T) {
	source := string(readArtifact(t, filepath.Join(dependencyModuleRoot(t, sdkModulePath), validatorDir), validatorFile))
	shapes := dispatch.WireShapes()

	var registered []reflect.Type
	for _, spec := range shapes.Unions() {
		registered = append(registered, spec.GoType)
	}
	for _, spec := range shapes.Constraints() {
		registered = append(registered, spec.GoType)
	}
	for _, spec := range shapes.ValueConstraints() {
		registered = append(registered, spec.GoType)
	}
	file, err := parser.ParseFile(token.NewFileSet(), validatorFile, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	owners := make(map[string]bool)
	for _, declaration := range file.Decls {
		method, ok := declaration.(*ast.FuncDecl)
		if !ok || method.Recv == nil || method.Name.Name != "validateWire" {
			continue
		}
		typ := method.Recv.List[0].Type
		if pointer, ok := typ.(*ast.StarExpr); ok {
			typ = pointer.X
		}
		if named, ok := typ.(*ast.Ident); ok {
			owners[named.Name] = true
		}
	}
	for _, shape := range registered {
		if !owners[shape.Name()] {
			t.Errorf("%s is registered as a wire-constrained shape without a validator", shape.Name())
		}
	}
}

// TestProtocolVersionAgreesEverywhere checks every machine-readable statement of
// the current protocol version against the code owner.
func TestProtocolVersionAgreesEverywhere(t *testing.T) {
	root := moduleRoot(t)

	var manifest struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "contract", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if unmarshalErr := json.Unmarshal(raw, &manifest); unmarshalErr != nil {
		t.Fatalf("decode manifest: %v", unmarshalErr)
	}
	if manifest.ProtocolVersion != protocol.Version {
		t.Errorf("manifest protocol %q != code %q", manifest.ProtocolVersion, protocol.Version)
	}

	// Canonical samples are another published statement. A client copies them
	// — that is what a canonical sample is for — so one naming a retired version
	// hands out a handshake the runtime refuses. The shape gate cannot see it: the
	// schema constrains the field's type, never which date is current.
	//
	// The client's own constant is NOT read here: it is generated from the same
	// code, so gate 1 already holds it. Test fixtures are not read either — a
	// refusal test has to be able to name a version this build rejects.
	samples, err := filepath.Glob(filepath.Join(root, "contract", "samples", "*.json"))
	if err != nil {
		t.Fatalf("glob canonical samples: %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("no canonical samples found; the sweep below would pass on nothing")
	}
	stated := 0
	for _, path := range samples {
		var decoded any
		if err := json.Unmarshal(readArtifact(t, filepath.Dir(path), filepath.Base(path)), &decoded); err != nil {
			t.Fatalf("decode %s: %v", filepath.Base(path), err)
		}
		for _, version := range collectProtocolVersions(decoded) {
			stated++
			if version != protocol.Version {
				t.Errorf("canonical sample %s states protocol version %q, which this build does not serve",
					filepath.Base(path), version)
			}
		}
	}
	if stated == 0 {
		t.Error("no canonical sample states a protocol version; the handshake fixtures stopped covering it")
	}
}

// TestProtocolContractIsRuntimeOwned prevents a consuming application from
// becoming the Runtime's contract author again. Clients may vendor generated
// bindings, but production code, generators and current architecture guidance
// must derive the contract from the Runtime registry; SDK/TypeScript copies
// are generated projections, never a second author.
func TestProtocolContractIsRuntimeOwned(t *testing.T) {
	root := moduleRoot(t)
	leaks := []string{
		strings.Join([]string{"desktop", "docs", "protocol"}, "/"),
		strings.Join([]string{`filepath.Join(root, "`, `..", "`, `desktop"`}, ""),
	}
	for _, rel := range []string{"README.md", "cmd", "internal", "docs/README.md", "docs/ARCHITECTURE.md"} {
		path := filepath.Join(root, rel)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		paths := []string{path}
		if info.IsDir() {
			paths = nil
			err := filepath.WalkDir(path, func(candidate string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !entry.IsDir() && slices.Contains([]string{".go", ".md"}, filepath.Ext(candidate)) {
					paths = append(paths, candidate)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walk %s: %v", rel, err)
			}
		}
		for _, candidate := range paths {
			source, err := os.ReadFile(candidate)
			if err != nil {
				t.Fatalf("read %s: %v", candidate, err)
			}
			for _, leaked := range leaks {
				if bytes.Contains(source, []byte(leaked)) {
					t.Errorf("%s resolves the Runtime protocol through a client module (%q)", candidate, leaked)
				}
			}
		}
	}
}

// protocolVersionKeys are the wire's only spellings of a protocol version.
var protocolVersionKeys = []string{"protocolVersion"}

// collectProtocolVersions walks decoded JSON for values stated under those keys, so
// a sample added later is swept without anyone remembering to list it.
func collectProtocolVersions(node any) []string {
	switch typed := node.(type) {
	case map[string]any:
		var out []string
		for key, value := range typed {
			if version, ok := value.(string); ok && slices.Contains(protocolVersionKeys, key) {
				out = append(out, version)
				continue
			}
			out = append(out, collectProtocolVersions(value)...)
		}
		return out
	case []any:
		var out []string
		for _, value := range typed {
			out = append(out, collectProtocolVersions(value)...)
		}
		return out
	default:
		return nil
	}
}

// TestGeneratedSchemasResolve checks that the OpenRPC document and
// the JSON Schema bundle parse, and every reference in them lands on a definition
// that exists.
//
// A dangling reference is the failure mode that matters here, because it is
// invisible to the drift gate: a stale generator produces a self-consistent pair
// of files and byte-compares clean forever. What it cannot survive is being
// resolved. The check is written here rather than delegated to a validator library
// so it holds on a machine with no network and no vendored schema tooling.
func TestGeneratedSchemasResolve(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "contract")

	var bundle struct {
		Schema string                    `json:"$schema"`
		Defs   map[string]jsontext.Value `json:"$defs"`
	}
	if err := json.Unmarshal(readArtifact(t, dir, "schema.json"), &bundle); err != nil {
		t.Fatalf("decode schema.json: %v", err)
	}
	if bundle.Schema == "" {
		t.Error("schema.json states no dialect; a schema whose dialect is a guess is not a contract")
	}
	if len(bundle.Defs) == 0 {
		t.Fatal("schema.json defines no types")
	}

	var openrpc struct {
		Components struct {
			Schemas map[string]jsontext.Value `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(readArtifact(t, dir, "openrpc.json"), &openrpc); err != nil {
		t.Fatalf("decode OpenRPC request components: %v", err)
	}
	referenced := make(map[string]bool)
	requestReferenced := make(map[string]bool)
	for _, document := range []struct {
		name   string
		prefix string
	}{
		{"schema.json", "#/$defs/"},
		{"openrpc.json", "schema.json#/$defs/"},
		{"manifest.json", "schema.json#/$defs/"},
	} {
		var decoded any
		if err := json.Unmarshal(readArtifact(t, dir, document.name), &decoded); err != nil {
			t.Fatalf("decode %s: %v", document.name, err)
		}
		refs := collectRefs(decoded)
		if len(refs) == 0 {
			t.Errorf("%s references no shapes at all", document.name)
		}
		for _, ref := range refs {
			if target, local := strings.CutPrefix(ref, "#/components/schemas/"); local && document.name == "openrpc.json" {
				if _, defined := openrpc.Components.Schemas[target]; !defined {
					t.Errorf("OpenRPC references undefined request component %q", target)
				} else {
					requestReferenced[target] = true
				}
				continue
			}
			target, ok := strings.CutPrefix(ref, document.prefix)
			if !ok {
				t.Errorf("%s references %q, which does not point into the shape bundle", document.name, ref)
				continue
			}
			if _, ok := bundle.Defs[target]; !ok {
				t.Errorf("%s references %q, which schema.json does not define", document.name, ref)
				continue
			}
			referenced[target] = true
		}
	}
	for name, encoded := range openrpc.Components.Schemas {
		if !requestReferenced[name] {
			t.Errorf("OpenRPC defines request component %q and nothing references it", name)
		}
		var component any
		if err := json.Unmarshal(encoded, &component); err != nil {
			t.Fatalf("decode request component %q: %v", name, err)
		}
		if !slices.Contains(collectRefs(component), "schema.json#/$defs/"+name) {
			t.Errorf("request component %q does not refine its reusable wire definition", name)
		}
	}

	// A definition nothing points at describes a frame no method can carry — either
	// a spec registered against an unreachable type, or a shape that outlived its
	// method.
	for name := range bundle.Defs {
		if !referenced[name] {
			t.Errorf("schema.json defines %q and nothing references it", name)
		}
	}
}

// TestOpenRPCDescribesEveryMethod pins the method document to the registry: the
// two artifacts are generated from one source, so a gap here means a method was
// projected into one artifact and not the other.
func TestOpenRPCDescribesEveryMethod(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "contract")

	var document struct {
		OpenRPC string `json:"openrpc"`
		Info    struct {
			Version string `json:"version"`
		} `json:"info"`
		Methods []struct {
			Name           string `json:"name"`
			ParamStructure string `json:"paramStructure"`
			Kind           string `json:"x-flame-kind"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(readArtifact(t, dir, "openrpc.json"), &document); err != nil {
		t.Fatalf("decode openrpc.json: %v", err)
	}
	if document.OpenRPC == "" {
		t.Error("openrpc.json states no spec version")
	}
	if document.Info.Version != protocol.Version {
		t.Errorf("openrpc.json is version %q, the code serves %q", document.Info.Version, protocol.Version)
	}

	var manifest struct {
		Methods []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"methods"`
	}
	if err := json.Unmarshal(readArtifact(t, dir, "manifest.json"), &manifest); err != nil {
		t.Fatalf("decode manifest.json: %v", err)
	}
	if len(document.Methods) != len(manifest.Methods) {
		t.Fatalf("openrpc.json has %d methods, the manifest has %d", len(document.Methods), len(manifest.Methods))
	}
	for index, method := range manifest.Methods {
		described := document.Methods[index]
		switch {
		case described.Name != method.Name:
			t.Errorf("method %d: openrpc says %q, the manifest says %q", index, described.Name, method.Name)
		case described.Kind != method.Kind:
			t.Errorf("%s: openrpc says %q, the manifest says %q", method.Name, described.Kind, method.Kind)
		case described.ParamStructure != "by-name":
			t.Errorf("%s: params are %q; the wire passes one object keyed by field name", method.Name, described.ParamStructure)
		}
	}
}

// collectRefs walks decoded JSON and returns every $ref value it finds.
func collectRefs(node any) []string {
	switch typed := node.(type) {
	case map[string]any:
		var out []string
		for key, value := range typed {
			if key == "$ref" {
				if ref, ok := value.(string); ok {
					out = append(out, ref)
				}
				continue
			}
			out = append(out, collectRefs(value)...)
		}
		return out
	case []any:
		var out []string
		for _, value := range typed {
			out = append(out, collectRefs(value)...)
		}
		return out
	default:
		return nil
	}
}

// notOnTheWire are the exported structs in the protocol package that the shape
// bundle is right to omit, each with the reason.
//
// It is a closed list on purpose. Everything else in that package IS the wire, so
// a new exported struct either appears in the bundle or has to be named here — the
// same discipline the union field-coverage check applies one level down. Without
// it, the way a shape goes unpublished is silently: nobody notices that no method
// reaches it.
var notOnTheWire = map[string]string{
	"WireField":              "Go reflection metadata for the Runtime DTO projection, never transported",
	"Failure":                "Go error carrier; ProblemData owns its wire projection",
	"SubscriptionOptions":    "Go call metadata projected to headers and request metadata",
	"RunSubscriptionOptions": "Go call metadata projected to headers and request metadata",
	"RunCommandOptions":      "Go call metadata projected to headers and request metadata",
	"PluginCallOptions":      "Go call metadata projected to headers and request metadata",
	"CommandOptions":         "Go call metadata projected to headers and request metadata",
	"CallOptions":            "Go call metadata projected to headers and request metadata",
	"Feature":                "the published capability vocabulary's registry entry; its wire projection is FeatureCapability",
	"ConstraintError":        "the Go validator's error carrier; its wire projection is ProblemData.errors",
	"PageContinuation":       "a composed value owner flattened into every Page instantiation, never a standalone JSON frame",
	"PageQuery":              "a composed cursor-query owner flattened into each paginated request, never a standalone JSON frame",
	"RecordedUsage":          "a composed usage block flattened into each usage report, never a standalone JSON frame",
}

// TestEveryWireStructIsPublished checks the bundle against the protocol package.
//
// The walk starts from the registered methods, which means a shape reachable from
// nothing — a tool result's members, an envelope member — is absent from every
// artifact and nothing complains. That is how the contract quietly loses a type a
// client still has to render, so the declaration mechanism exists (carried shapes,
// state payloads) and this proves it was used.
func TestEveryWireStructIsPublished(t *testing.T) {
	root := moduleRoot(t)

	var bundle struct {
		Defs map[string]jsontext.Value `json:"$defs"`
	}
	if err := json.Unmarshal(readArtifact(t, filepath.Join(root, "contract"), "schema.json"), &bundle); err != nil {
		t.Fatalf("decode schema.json: %v", err)
	}
	// A generic publishes as one definition per instantiation (PageOfSession), so the
	// base name counts as published.
	published := make(map[string]bool, len(bundle.Defs))
	for name := range bundle.Defs {
		published[name] = true
		if base, _, generic := strings.Cut(name, "Of"); generic {
			published[base] = true
		}
	}

	for _, name := range exportedStructs(t, filepath.Join(dependencyModuleRoot(t, sdkModulePath), "protocol")) {
		reason, excused := notOnTheWire[name]
		switch {
		case published[name] && excused:
			t.Errorf("%s is published AND listed as not on the wire (%q) — one of the two is wrong", name, reason)
		case !published[name] && !excused:
			t.Errorf("%s is a wire struct no artifact describes; register how it is carried, or say why it is not on the wire", name)
		}
	}
	for name := range notOnTheWire {
		if !slices.Contains(exportedStructs(t, filepath.Join(dependencyModuleRoot(t, sdkModulePath), "protocol")), name) {
			t.Errorf("%s is excused from the bundle and no longer exists", name)
		}
	}
}

func exportedStructs(t *testing.T, dir string) []string {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	var out []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}
			for _, spec := range group.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !typeSpec.Name.IsExported() {
					continue
				}
				if _, ok := typeSpec.Type.(*ast.StructType); ok {
					out = append(out, typeSpec.Name.Name)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

// Flattened owners have no standalone schema. Their rules must survive in
// every published carrier, including nested paths and conditional branches.
func TestAllValueConstraintsReachTheirSchemaOwners(t *testing.T) {
	var bundle struct {
		Defs map[string]jsontext.Value `json:"$defs"`
	}
	if err := json.Unmarshal(readArtifact(t, filepath.Join(moduleRoot(t), "contract"), "schema.json"), &bundle); err != nil {
		t.Fatal(err)
	}
	for _, spec := range dispatch.WireShapes().ValueConstraints() {
		shape := spec.GoType.Name()
		for _, rule := range spec.Constraints {
			keyword := constraintKeyword(t, spec.GoType, rule)
			owners := 0
			for name, definition := range bundle.Defs {
				if _, present := bundle.Defs[shape]; present {
					if name != shape {
						continue
					}
				} else {
					if _, ok := notOnTheWire[shape]; !ok {
						t.Fatalf("schema missing for %s", shape)
					}
					if !schemaHasTopLevelProperty(t, definition, rule.Field) {
						continue
					}
				}
				owners++
				if !constraintInSchema(t, definition, rule.Field, keyword, rule) {
					t.Errorf("%s.%s missing %s", name, rule.Field, keyword)
				}
			}
			if owners == 0 {
				t.Errorf("%s.%s has no published carrier", shape, rule.Field)
			}
		}
	}
}

func constraintKeyword(t *testing.T, typ reflect.Type, rule dispatch.FieldConstraint) string {
	t.Helper()
	switch rule.Kind {
	case dispatch.ConstraintNonEmpty:
		return "minLength"
	case dispatch.ConstraintPositive:
		_, field, ok := protocol.WireGoPath(typ, rule.Field)
		if !ok {
			t.Fatalf("%s.%s is not a wire field", typ.Name(), rule.Field)
		}
		if protocol.WireBaseType(field.Type).Kind() == reflect.Float64 {
			return "exclusiveMinimum"
		}
		return "minimum"
	case dispatch.ConstraintNonNegative, dispatch.ConstraintMinimum:
		return "minimum"
	case dispatch.ConstraintMaximum:
		return "maximum"
	case dispatch.ConstraintNonEmptyItems, dispatch.ConstraintMinItems:
		return "minItems"
	case dispatch.ConstraintNonEmptyProperties:
		return "minProperties"
	case dispatch.ConstraintUniqueItems:
		return "uniqueItems"
	case dispatch.ConstraintMaxItems:
		return "maxItems"
	case dispatch.ConstraintMaxLength, dispatch.ConstraintMaxItemLength, dispatch.ConstraintMaxPropertyNameLength:
		return "maxLength"
	case dispatch.ConstraintIdentity, dispatch.ConstraintIdentityItems, dispatch.ConstraintIdentityPropertyNames, dispatch.ConstraintPrefix, dispatch.ConstraintPrefixItems, dispatch.ConstraintPatternItems, dispatch.ConstraintPattern:
		return "pattern"
	default:
		t.Fatalf("unsupported wire constraint %v", rule)
		return ""
	}
}

func TestCompiledWireValueBoundaries(t *testing.T) {
	var vectors []struct {
		Shape string         `json:"shape"`
		Value jsontext.Value `json:"value"`
		Valid bool           `json:"valid"`
	}
	if err := json.Unmarshal(readArtifact(t, filepath.Join(moduleRoot(t), "internal", "arch", "testdata"), "wire-values.json"), &vectors); err != nil {
		t.Fatal(err)
	}
	shapes := make(map[string]reflect.Type)
	for _, spec := range dispatch.WireShapes().ValueConstraints() {
		shapes[spec.GoType.Name()] = spec.GoType
	}
	for _, v := range vectors {
		typ := shapes[v.Shape]
		if typ == nil {
			t.Fatalf("unknown constrained shape %s", v.Shape)
		}
		value := reflect.New(typ).Interface()
		if err := json.Unmarshal(v.Value, value); err != nil {
			t.Fatal(err)
		}
		if err := protocol.ValidateWireTree(value); (err == nil) != v.Valid {
			t.Errorf("%s %s: %v", v.Shape, v.Value, err)
		}
	}
}

func schemaHasTopLevelProperty(t *testing.T, definition jsontext.Value, field string) bool {
	t.Helper()
	var object struct {
		Properties map[string]jsontext.Value `json:"properties"`
	}
	if err := json.Unmarshal(definition, &object); err != nil {
		t.Fatalf("decode definition: %v", err)
	}
	_, exists := object.Properties[field]
	return exists
}

// constraintInSchema reports whether the definition constrains the last segment of
// a dotted path with the given keyword, at any depth.
func constraintInSchema(t *testing.T, definition jsontext.Value, path, keyword string, constraint dispatch.FieldConstraint) bool {
	t.Helper()

	var decoded any
	if err := json.Unmarshal(definition, &decoded); err != nil {
		t.Fatalf("decode definition: %v", err)
	}
	segments := strings.Split(path, ".")
	return statesKeyword(decoded, segments[len(segments)-1], keyword, constraint)
}

func statesKeyword(node any, property, keyword string, constraint dispatch.FieldConstraint) bool {
	switch typed := node.(type) {
	case map[string]any:
		if mapStatesPropertyConstraint(typed, property, keyword, constraint) {
			return true
		}
		for _, value := range typed {
			if statesKeyword(value, property, keyword, constraint) {
				return true
			}
		}
	case []any:
		for _, value := range typed {
			if statesKeyword(value, property, keyword, constraint) {
				return true
			}
		}
	}
	return false
}

func mapStatesPropertyConstraint(node map[string]any, property, keyword string, constraint dispatch.FieldConstraint) bool {
	properties, hasProperties := node["properties"].(map[string]any)
	if !hasProperties {
		return false
	}
	constrained, hasProperty := properties[property].(map[string]any)
	if !hasProperty {
		return false
	}
	if constraint.Kind == dispatch.ConstraintMaxItemLength ||
		constraint.Kind == dispatch.ConstraintIdentityItems ||
		constraint.Kind == dispatch.ConstraintPrefixItems ||
		constraint.Kind == dispatch.ConstraintPatternItems {
		constrained, hasProperty = constrained["items"].(map[string]any)
		if !hasProperty {
			return false
		}
	}
	if constraint.Kind == dispatch.ConstraintMaxPropertyNameLength ||
		constraint.Kind == dispatch.ConstraintIdentityPropertyNames {
		constrained, hasProperty = constrained["propertyNames"].(map[string]any)
		if !hasProperty {
			return false
		}
	}
	return schemaNodeStatesConstraint(constrained, keyword, constraint)
}

// schemaNodeStatesConstraint walks an individual property's conjunctive schema.
// A JSON Schema node owns only one pattern keyword, so independently declared
// predicates such as an identity alphabet and a required frame prefix compile
// into sibling allOf nodes rather than overwriting one another.
func schemaNodeStatesConstraint(node any, keyword string, constraint dispatch.FieldConstraint) bool {
	switch typed := node.(type) {
	case map[string]any:
		if value, stated := typed[keyword]; stated && constraintValueMatches(value, constraint) {
			return true
		}
		for _, child := range typed {
			if schemaNodeStatesConstraint(child, keyword, constraint) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if schemaNodeStatesConstraint(child, keyword, constraint) {
				return true
			}
		}
	}
	return false
}

func constraintValueMatches(value any, constraint dispatch.FieldConstraint) bool {
	if constraint.Kind == dispatch.ConstraintPattern ||
		constraint.Kind == dispatch.ConstraintPatternItems {
		text, isText := value.(string)
		return isText && text == constraint.Value
	}
	if constraint.Value != "" {
		text, isText := value.(string)
		return isText && text == "^"+regexp.QuoteMeta(constraint.Value)
	}
	if constraint.Kind == dispatch.ConstraintIdentity ||
		constraint.Kind == dispatch.ConstraintIdentityItems ||
		constraint.Kind == dispatch.ConstraintIdentityPropertyNames {
		text, isText := value.(string)
		return isText && text == dispatch.IdentityPattern
	}
	if constraint.Limit == 0 {
		return true
	}
	number, isNumber := value.(float64)
	return isNumber && number == float64(constraint.Limit)
}
