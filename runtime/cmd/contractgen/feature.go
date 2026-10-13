package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/capability"
)

type featureMethod struct {
	Name        string                `json:"name"`
	Kind        capability.MethodKind `json:"kind"`
	Target      string                `json:"target"`
	Description string                `json:"description"`
	Input       string                `json:"input"`
	Output      string                `json:"output"`
}

type featureArtifacts struct {
	definitions map[string]jsontext.Value
	methods     []featureMethod
	typescript  string
	capability  *capability.Contract
}

func runFeature(source, dir, tsDir string, goOutputs ...featureGoOutput) error {
	if source != "-" {
		if err := protectFeatureSource(source, dir, tsDir); err != nil {
			return err
		}
	}
	var data []byte
	var err error
	if source == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(source)
	}
	if err != nil {
		return err
	}
	artifacts, err := buildFeature(data)
	if err != nil {
		return err
	}
	goBodies := make(map[string][]byte, len(goOutputs))
	for _, output := range goOutputs {
		if output.Path == "" {
			continue
		}
		if source != "-" {
			input, err := filepath.Abs(source)
			if err != nil {
				return err
			}
			target, err := filepath.Abs(output.Path)
			if err != nil {
				return err
			}
			if input == target {
				return fmt.Errorf("feature Go output would overwrite input")
			}
			original, err := os.Stat(source)
			if err != nil {
				return err
			}
			if existing, err := os.Stat(output.Path); err == nil && os.SameFile(original, existing) {
				return fmt.Errorf("feature Go output would overwrite input")
			}
		}
		body, err := buildFeatureGo(artifacts, output)
		if err != nil {
			return err
		}
		goBodies[output.Path] = body
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "schema.json"), struct {
		Schema string                    `json:"$schema"`
		Defs   map[string]jsontext.Value `json:"$defs"`
	}{schemaDialect, artifacts.definitions}); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "methods.generated.json"), artifacts.methods); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "capability.generated.json"), artifacts.capability.Descriptor()); err != nil {
		return err
	}
	var reference strings.Builder
	reference.WriteString("# Feature operations\n\nGenerated from the feature's Go-owned method contracts.\n\n| Method | Kind | Target | Description |\n| --- | --- | --- | --- |\n")
	for _, method := range artifacts.methods {
		description := strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(method.Description)
		fmt.Fprintf(&reference, "| `%s` | %s | %s | %s |\n", method.Name, method.Kind, method.Target, description)
	}
	descriptor := artifacts.capability.Descriptor()
	fmt.Fprintf(&reference, "\nCapability `%s` uses `%s` composition in `%s` scope. Required backend protocol: `%s`.\n\n`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.\n", descriptor.Reference, descriptor.Composition, descriptor.Scope, descriptor.Protocol)
	if err := os.WriteFile(filepath.Join(dir, "API_REFERENCE.md"), []byte(reference.String()), 0644); err != nil {
		return err
	}
	if tsDir != "" {
		if err := os.MkdirAll(tsDir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(tsDir, "methods.generated.ts"), []byte(artifacts.typescript), 0644); err != nil {
			return err
		}
	}

	for _, output := range goOutputs {
		if output.Path == "" {
			continue
		}
		body := goBodies[output.Path]
		if err := os.MkdirAll(filepath.Dir(output.Path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(output.Path, body, 0644); err != nil {
			return err
		}
	}

	return nil
}

func protectFeatureSource(source, dir, tsDir string) error {
	sourcePath, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return err
	}
	outputs := []string{filepath.Join(dir, "schema.json"), filepath.Join(dir, "methods.generated.json"), filepath.Join(dir, "capability.generated.json"), filepath.Join(dir, "API_REFERENCE.md")}
	if tsDir != "" {
		outputs = append(outputs, filepath.Join(tsDir, "methods.generated.ts"))
	}
	for _, output := range outputs {
		outputPath, err := filepath.Abs(output)
		if err != nil {
			return err
		}
		info, err := os.Stat(output)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if sourcePath == outputPath || err == nil && os.SameFile(sourceInfo, info) {
			return fmt.Errorf("feature contracts: output would overwrite descriptor input %s", source)
		}
	}
	return nil
}

func buildFeature(data []byte) (featureArtifacts, error) {
	contract, err := capability.Parse(data)
	if err != nil {
		return featureArtifacts{}, err
	}
	descriptor := contract.Descriptor()
	descriptors := descriptor.Methods
	artifacts := featureArtifacts{definitions: make(map[string]jsontext.Value), capability: contract}
	set := &schemaSet{defs: make(map[string]*schema)}
	for _, descriptor := range descriptors {
		if descriptor.Description == "" || !utf8.ValidString(descriptor.Description) {
			return featureArtifacts{}, fmt.Errorf("feature contracts: method description is required for %q", descriptor.Name)
		}
		root := featureTypeName(descriptor.Name)
		method := featureMethod{Name: descriptor.Name, Kind: descriptor.Kind, Target: descriptor.Target, Description: descriptor.Description, Input: "schema.json" + refPrefix + root + "Input", Output: "schema.json" + refPrefix + root + "Output"}
		for _, frame := range []struct {
			name string
			raw  jsontext.Value
		}{{root + "Input", descriptor.Input}, {root + "Output", descriptor.Output}} {
			if err := addFeatureSchema(&artifacts, frame.name, frame.raw); err != nil {
				return featureArtifacts{}, err
			}
		}
		artifacts.methods = append(artifacts.methods, method)
	}
	for name, raw := range artifacts.definitions {
		node, err := featureNode(raw)
		if err != nil {
			return featureArtifacts{}, fmt.Errorf("feature contracts: %s: %w", name, err)
		}
		set.defs[name] = node
	}
	emitter := &tsEmitter{tsTypes: tsTypes{set: set}}
	emitter.line("// Code generated by Flame contractgen from Go-owned feature methods. DO NOT EDIT.")
	emitter.line("// Numeric and cross-field constraints remain in schema.json and the business owner.")
	emitter.line("")
	for _, name := range slices.Sorted(maps.Keys(set.defs)) {
		emitter.define(name, set.defs[name])
	}
	emitter.line("export interface MethodInputs {")
	for _, method := range artifacts.methods {
		emitter.line("  %s: %s;", strconv.Quote(method.Name), strings.TrimPrefix(method.Input, "schema.json"+refPrefix))
	}
	emitter.line("}")
	emitter.line("export interface MethodOutputs {")
	for _, method := range artifacts.methods {
		emitter.line("  %s: %s;", strconv.Quote(method.Name), strings.TrimPrefix(method.Output, "schema.json"+refPrefix))
	}
	emitter.line("}")
	emitter.line("export const METHODS = {")
	for _, method := range artifacts.methods {
		emitter.line("  %s: { kind: %s, target: %s },", strconv.Quote(method.Name), strconv.Quote(string(method.Kind)), strconv.Quote(method.Target))
	}
	emitter.line("} as const;")
	emitter.line("export const CAPABILITY = { protocol: %s, reference: %s, composition: %s, scope: %s, contract: %s } as const;", strconv.Quote(descriptor.Protocol), strconv.Quote(descriptor.Reference), strconv.Quote(string(descriptor.Composition)), strconv.Quote(descriptor.Scope), strconv.Quote(contract.Identity().Digest()))
	emitter.line("export const CAPABILITY_JSON = %s;", strconv.Quote(string(contract.Identity().JSON())))
	artifacts.typescript = emitter.out.String()
	return artifacts, nil
}

// Names are identifiers inside generated files, never filesystem components.
// Hex escapes preserve distinct semantic names rather than silently colliding.
func featureTypeName(name string) string {
	var out strings.Builder
	out.WriteString("Method_")
	for _, value := range []byte(name) {
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' {
			out.WriteByte(value)
		} else {
			fmt.Fprintf(&out, "_%02x", value)
		}
	}
	return out.String()
}

func addFeatureSchema(artifacts *featureArtifacts, root string, raw jsontext.Value) error {
	var document map[string]jsontext.Value
	if err := json.Unmarshal(raw, &document); err != nil {
		return err
	}
	var definitions map[string]jsontext.Value
	if defs := document["$defs"]; len(defs) != 0 {
		if err := json.Unmarshal(defs, &definitions); err != nil {
			return err
		}
	}
	delete(document, "$defs")
	delete(document, "$schema")
	renamed := make(map[string]string)
	for name := range definitions {
		renamed[name] = featureTypeName(name)
	}
	for name, body := range definitions {
		updated, err := rewriteFeatureRefs(body, renamed)
		if err != nil {
			return err
		}
		if previous, exists := artifacts.definitions[renamed[name]]; exists && string(previous) != string(updated) {
			return fmt.Errorf("feature contracts: definition %q has competing schemas", name)
		}
		artifacts.definitions[renamed[name]] = updated
	}
	body, err := json.Marshal(document, json.Deterministic(true))
	if err != nil {
		return err
	}
	updated, err := rewriteFeatureRefs(body, renamed)
	if err != nil {
		return err
	}
	// Scope expands the root while retaining its named definition. Publish one
	// data shape when that definition is unambiguous; method identities remain distinct.
	var matching []string
	for name := range definitions {
		if string(artifacts.definitions[renamed[name]]) == string(updated) {
			matching = append(matching, renamed[name])
		}
	}
	if len(matching) == 1 {
		updated, err = json.Marshal(map[string]string{"$ref": refPrefix + matching[0]})
		if err != nil {
			return err
		}
	}
	artifacts.definitions[root] = updated
	return nil
}

func rewriteFeatureRefs(raw jsontext.Value, renamed map[string]string) (jsontext.Value, error) {
	if string(raw) == "true" || string(raw) == "false" {
		return raw, nil
	}
	var node map[string]jsontext.Value
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, err
	}
	if reference := node["$ref"]; len(reference) != 0 {
		var ref string
		if err := json.Unmarshal(reference, &ref); err != nil {
			return nil, err
		}
		name, local := strings.CutPrefix(ref, refPrefix)
		if !local || renamed[name] == "" {
			return nil, fmt.Errorf("feature contracts: unsupported reference %q", ref)
		}
		encoded, err := json.Marshal(refPrefix + renamed[name])
		if err != nil {
			return nil, err
		}
		node["$ref"] = encoded
	}
	for _, key := range []string{"properties", "patternProperties", "dependentSchemas", "$defs"} {
		if len(node[key]) == 0 {
			continue
		}
		var children map[string]jsontext.Value
		if err := json.Unmarshal(node[key], &children); err != nil {
			return nil, err
		}
		for name, child := range children {
			updated, err := rewriteFeatureRefs(child, renamed)
			if err != nil {
				return nil, err
			}
			children[name] = updated
		}
		encoded, err := json.Marshal(children, json.Deterministic(true))
		if err != nil {
			return nil, err
		}
		node[key] = encoded
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		if len(node[key]) == 0 {
			continue
		}
		var children []jsontext.Value
		if err := json.Unmarshal(node[key], &children); err != nil {
			return nil, err
		}
		for index, child := range children {
			updated, err := rewriteFeatureRefs(child, renamed)
			if err != nil {
				return nil, err
			}
			children[index] = updated
		}
		encoded, err := json.Marshal(children)
		if err != nil {
			return nil, err
		}
		node[key] = encoded
	}
	for _, key := range []string{"items", "additionalProperties", "unevaluatedProperties", "propertyNames", "contains", "not", "if", "then", "else"} {
		if len(node[key]) == 0 {
			continue
		}
		updated, err := rewriteFeatureRefs(node[key], renamed)
		if err != nil {
			return nil, err
		}
		node[key] = updated
	}
	return json.Marshal(node, json.Deterministic(true))
}

// This adapter reads only type-level keywords for the existing TS emitter.
// The complete standard Schema, including numeric constraints, is retained above.
func featureNode(raw jsontext.Value) (*schema, error) {
	if string(raw) == "false" {
		return &schema{TypeScriptType: "never"}, nil
	}
	if string(raw) == "true" {
		return &schema{}, nil
	}
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	node := &schema{}
	var multipleTypes []schemaType
	for key, target := range map[string]any{"$ref": &node.Ref, "required": &node.Required} {
		if len(fields[key]) != 0 {
			if err := json.Unmarshal(fields[key], target); err != nil {
				return nil, err
			}
		}
	}
	if value := fields["type"]; len(value) != 0 {
		if err := json.Unmarshal(value, &node.Type); err != nil {
			if err := json.Unmarshal(value, &multipleTypes); err != nil {
				return nil, err
			}
		}
	}
	if value := fields["const"]; len(value) != 0 {
		node.TypeScriptType = string(value)
	}
	if value := fields["enum"]; len(value) != 0 {
		var values []jsontext.Value
		if err := json.Unmarshal(value, &values); err != nil {
			return nil, err
		}
		parts := make([]string, len(values))
		for index, value := range values {
			parts[index] = string(value)
		}
		node.TypeScriptType = unionOf(parts)
	}
	if value := fields["properties"]; len(value) != 0 {
		var children map[string]jsontext.Value
		if err := json.Unmarshal(value, &children); err != nil {
			return nil, err
		}
		node.Properties = make(map[string]any)
		for name, raw := range children {
			child, err := featureNode(raw)
			if err != nil {
				return nil, err
			}
			node.Properties[name] = child
		}
	}
	if len(fields["items"]) != 0 {
		child, err := featureNode(fields["items"])
		if err != nil {
			return nil, err
		}
		node.Items = child
	}
	if raw := fields["additionalProperties"]; len(raw) != 0 && string(raw) != "false" {
		child, err := featureNode(raw)
		if err != nil {
			return nil, err
		}
		node.AdditionalProps = child
	}
	for key, target := range map[string]*[]*schema{"anyOf": &node.AnyOf, "oneOf": &node.OneOf} {
		if len(fields[key]) == 0 {
			continue
		}
		var children []jsontext.Value
		if err := json.Unmarshal(fields[key], &children); err != nil {
			return nil, err
		}
		for _, raw := range children {
			child, err := featureNode(raw)
			if err != nil {
				return nil, err
			}
			*target = append(*target, child)
		}
	}
	if len(multipleTypes) != 0 {
		for _, kind := range multipleTypes {
			member := *node
			member.Type = kind
			member.AnyOf = nil
			node.AnyOf = append(node.AnyOf, &member)
		}
	}
	return node, nil
}
