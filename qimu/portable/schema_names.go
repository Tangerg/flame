package portable

import (
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
	"unicode"

	"github.com/Tangerg/scope/core/jsonschema"
)

const definitionRef = "#/$defs/"

// ReadableDefinitions renames the definitions a Go schema reflector keys as
// go_<base64url(package path)>_<type> to <package>.<type> — the package's last
// path element, with any '.' in it replaced by '_' so the first '.' separates
// the two — and rewrites every reference. The name depends on the type alone, so
// every method and contract publishes one type under one name; the reflector's
// encoding only existed to keep names unique. A name that still collides is
// rejected rather than escaped into an unreadable one.
func ReadableDefinitions(schema []byte) ([]byte, error) {
	var document map[string]jsontext.Value
	if err := json.Unmarshal(schema, &document); err != nil {
		return nil, fmt.Errorf("portable: schema: %w", err)
	}
	encodedDefinitions, found := document["$defs"]
	if !found {
		return bytes.Clone(schema), nil
	}
	var definitions map[string]jsontext.Value
	if err := json.Unmarshal(encodedDefinitions, &definitions); err != nil {
		return nil, fmt.Errorf("portable: schema definitions: %w", err)
	}
	renamed, err := readableNames(slices.Collect(maps.Keys(definitions)))
	if err != nil {
		return nil, err
	}
	rewrite := func(value jsontext.Value) jsontext.Value {
		for from, to := range renamed {
			value = bytes.ReplaceAll(value, []byte(`"`+definitionRef+from+`"`), []byte(`"`+definitionRef+to+`"`))
		}
		return value
	}
	readable := make(map[string]jsontext.Value, len(definitions))
	for key, body := range definitions {
		readable[renamed[key]] = rewrite(body)
	}
	encoded, err := json.Marshal(readable, json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	for key, value := range document {
		document[key] = rewrite(value)
	}
	document["$defs"] = encoded
	return json.Marshal(document, json.Deterministic(true))
}

func readableNames(keys []string) (map[string]string, error) {
	renamed := make(map[string]string, len(keys))
	owners := make(map[string]string, len(keys))
	for _, key := range slices.Sorted(slices.Values(keys)) {
		name := key
		if pkg, typeName := goDefinitionSource(key); pkg != "" {
			name = strings.ReplaceAll(path.Base(pkg), ".", "_") + "." + typeName
		}
		if previous, taken := owners[name]; taken {
			return nil, fmt.Errorf("portable: schema definitions %q and %q both read as %s", previous, key, name)
		}
		owners[name] = key
		renamed[key] = name
	}
	return renamed, nil
}

// goDefinitionSource reads the package path and type name from a reflector
// key; any other key is its own name. Base64url and Go type names may both
// contain '_', so the separator is the first one after which the prefix
// decodes to an import path and the rest begins a type name.
func goDefinitionSource(key string) (pkg, name string) {
	encoded, ok := strings.CutPrefix(key, "go_")
	if !ok {
		return "", key
	}
	for index := strings.IndexByte(encoded, '_'); index >= 0; {
		decoded, err := base64.RawURLEncoding.DecodeString(encoded[:index])
		rest := encoded[index+1:]
		if err == nil && importPath(string(decoded)) && rest != "" && unicode.IsLetter([]rune(rest)[0]) {
			return string(decoded), rest
		}
		next := strings.IndexByte(rest, '_')
		if next < 0 {
			break
		}
		index += next + 1
	}
	return "", key
}

func importPath(value string) bool {
	return value != "" && !strings.ContainsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("./-_~+", r))
	})
}

// SchemaFor derives T's JSON Schema with readable definition names. It is the
// one way Go types become schemas, so every contract, tool and point names a
// type the same way.
func SchemaFor[T any]() (jsonschema.Schema, error) {
	derived, err := jsonschema.For[T]()
	if err != nil {
		return jsonschema.Schema{}, err
	}
	readable, err := ReadableDefinitions(derived.JSON())
	if err != nil {
		return jsonschema.Schema{}, err
	}
	return jsonschema.Parse(readable)
}
