package main

import (
	"encoding/base64"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/frame"
	framevalue "github.com/Tangerg/frame/value"
)

const scopeDefinitionPrefix = "go_"

func projectFeatureNames(descriptor frame.CapabilityDescriptor) (frame.CapabilityDescriptor, error) {
	descriptor.Methods = slices.Clone(descriptor.Methods)
	for index := range descriptor.Methods {
		method := &descriptor.Methods[index]
		for _, schema := range []*jsontext.Value{&method.Input, &method.Output, &method.Stream} {
			if len(*schema) == 0 {
				continue
			}
			projected, err := projectSchemaNames(*schema)
			if err != nil {
				return frame.CapabilityDescriptor{}, fmt.Errorf("feature contracts: %s definition names: %w", method.Name, err)
			}
			*schema = projected
		}
	}
	return descriptor, nil
}

func projectSchemaNames(raw jsontext.Value) (jsontext.Value, error) {
	if raw.Kind() != '{' {
		return raw, nil
	}
	var schema map[string]jsontext.Value
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	var definitions map[string]jsontext.Value
	if encoded := schema[framevalue.KeywordDefs]; len(encoded) != 0 {
		if err := json.Unmarshal(encoded, &definitions); err != nil {
			return nil, err
		}
	}
	names := readableDefinitionNames(definitions)
	if len(names) == 0 {
		return raw, nil
	}
	projected := make(map[string]jsontext.Value, len(definitions))
	for name, body := range definitions {
		if replacement, found := names[name]; found {
			name = replacement
		}
		projected[name] = body
	}
	var err error
	schema[framevalue.KeywordDefs], err = json.Marshal(projected, json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	raw, err = json.Marshal(schema, json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	return (definitionReferences{names: names, profile: framevalue.ProfileSchema()}).rewrite(raw)
}

type definitionName struct {
	packageParts []string
	typeName     string
	depth        int
}

func scopeDefinitionName(key string) (definitionName, bool) {
	encoded, found := strings.CutPrefix(key, scopeDefinitionPrefix)
	if !found {
		return definitionName{}, false
	}
	packageToken, typeToken, found := strings.Cut(encoded, ".")
	if !found {
		return definitionName{}, false
	}
	var decoded [2]string
	for index, token := range []string{packageToken, typeToken} {
		value, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil || len(value) == 0 || !utf8.Valid(value) || base64.RawURLEncoding.EncodeToString(value) != token {
			return definitionName{}, false
		}
		decoded[index] = string(value)
	}
	return definitionName{packageParts: strings.Split(decoded[0], "/"), typeName: decoded[1], depth: 1}, true
}

func (n definitionName) candidate() string {
	qualifier := strings.Join(n.packageParts[len(n.packageParts)-n.depth:], "/")
	// Frame separates its package qualifier at the first dot. A domain name's
	// dots must not consume that separator or hide a private codec's first rune.
	return strings.ReplaceAll(qualifier, ".", "_") + "." + n.typeName
}

func readableDefinitionNames(definitions map[string]jsontext.Value) map[string]string {
	decoded := make(map[string]definitionName)
	reserved := make(map[string]bool)
	for key := range definitions {
		if name, found := scopeDefinitionName(key); found {
			decoded[key] = name
		} else {
			reserved[key] = true
		}
	}
	for {
		counts := make(map[string]int)
		for _, name := range decoded {
			counts[name.candidate()]++
		}
		expanded := false
		for key, name := range decoded {
			candidate := name.candidate()
			if (counts[candidate] > 1 || reserved[candidate]) && name.depth < len(name.packageParts) {
				name.depth++
				decoded[key] = name
				expanded = true
			}
		}
		if !expanded {
			break
		}
	}
	names := make(map[string]string, len(decoded))
	for _, key := range slices.Sorted(maps.Keys(decoded)) {
		preferred := decoded[key].candidate()
		name := preferred
		for suffix := 2; reserved[name]; suffix++ {
			name = preferred + strconv.Itoa(suffix)
		}
		names[key], reserved[name] = name, true
	}
	return names
}

type definitionReferences struct {
	names   map[string]string
	profile framevalue.SchemaProfile
}

func (r definitionReferences) rewrite(raw jsontext.Value) (jsontext.Value, error) {
	if raw.Kind() != '{' {
		return raw, nil
	}
	var schema map[string]jsontext.Value
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	if reference := schema[framevalue.KeywordRef]; len(reference) != 0 {
		projected, err := r.reference(reference)
		if err != nil {
			return nil, err
		}
		schema[framevalue.KeywordRef] = projected
	}
	for key, child := range schema {
		projected, err := r.children(child, r.profile.Keywords[key].Children)
		if err != nil {
			return nil, err
		}
		schema[key] = projected
	}
	return json.Marshal(schema, json.Deterministic(true))
}

func (r definitionReferences) reference(raw jsontext.Value) (jsontext.Value, error) {
	var reference string
	if err := json.Unmarshal(raw, &reference); err != nil {
		return nil, err
	}
	fragment, local := strings.CutPrefix(reference, "#")
	if !local {
		return raw, nil
	}
	fragment, err := url.PathUnescape(fragment)
	if err != nil {
		return nil, err
	}
	pointer := jsontext.Pointer(fragment)
	if !pointer.IsValid() {
		return nil, fmt.Errorf("invalid schema reference %q", reference)
	}
	tokens := slices.Collect(pointer.Tokens())
	if len(tokens) < 2 || tokens[0] != framevalue.KeywordDefs {
		return raw, nil
	}
	name, found := r.names[tokens[1]]
	if !found {
		return raw, nil
	}
	tokens[1] = name
	pointer = ""
	for _, token := range tokens {
		pointer = pointer.AppendToken(token)
	}
	projected := url.URL{Fragment: string(pointer)}
	return json.Marshal(projected.String())
}

func (r definitionReferences) children(raw jsontext.Value, kind framevalue.SchemaChildren) (jsontext.Value, error) {
	switch kind {
	case framevalue.SchemaChildrenSchema:
		return r.rewrite(raw)
	case framevalue.SchemaChildrenMap:
		var children map[string]jsontext.Value
		if err := json.Unmarshal(raw, &children); err != nil {
			return nil, err
		}
		for name, child := range children {
			projected, err := r.rewrite(child)
			if err != nil {
				return nil, err
			}
			children[name] = projected
		}
		return json.Marshal(children, json.Deterministic(true))
	case framevalue.SchemaChildrenArray:
		var children []jsontext.Value
		if err := json.Unmarshal(raw, &children); err != nil {
			return nil, err
		}
		for index, child := range children {
			projected, err := r.rewrite(child)
			if err != nil {
				return nil, err
			}
			children[index] = projected
		}
		return json.Marshal(children)
	default:
		return raw, nil
	}
}
