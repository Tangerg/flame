package portable

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Tangerg/scope/core/jsonschema"
)

// Schema owns profile admission and bounded evaluation preflight. Scope remains
// the compiler and the only owner of JSON Schema validation semantics.
type Schema struct {
	compiled jsonschema.Schema
	work     []int
	unique   bool
}

func AdmitSchema(raw []byte) (Schema, error) {
	if err := CheckDescriptor(raw); err != nil {
		return Schema{}, err
	}
	value, err := decodeBoundedJSON(raw)
	if err != nil {
		return Schema{}, err
	}
	work, unique, err := schemaWork(value)
	if err != nil {
		return Schema{}, err
	}
	compiled, err := jsonschema.Parse(raw)
	if err != nil {
		return Schema{}, err
	}
	return Schema{compiled: compiled, work: work, unique: unique}, nil
}

func (s Schema) Validate(raw []byte) error {
	if !s.compiled.Valid() || len(s.work) == 0 {
		return jsonschema.ErrInvalid
	}
	size, err := checkJSON(raw, 0)
	if err != nil {
		return err
	}
	if s.unique && size.equality > ProfileLimits().MaxEqualityWork {
		return limit("schema equality work", ProfileLimits().MaxEqualityWork)
	}
	// The precomputed bound explores every schema branch, independently of its
	// predicate. Object names also count because propertyNames can validate them.
	maximum := ProfileLimits().MaxValidationWork
	if s.work[size.depth] > maximum/(size.nodes+size.keys) {
		return limit("schema validation work", maximum)
	}
	return s.compiled.Validate(raw)
}

type schemaNode struct {
	same, descend []int
	local         int
	unique        bool
}

type schemaReference struct {
	source int
	path   string
}

var schemaAnnotations = map[string]bool{
	"$schema": true, "$comment": true, "title": true, "description": true,
	"default": true, "examples": true, "readOnly": true, "writeOnly": true,
	"deprecated": true, "contentEncoding": true, "contentMediaType": true,
	"type": true, "enum": true, "const": true, "required": true,
	"minimum": true, "maximum": true, "exclusiveMinimum": true,
	"exclusiveMaximum": true, "multipleOf": true, "minLength": true,
	"maxLength": true, "pattern": true, "format": true, "minItems": true,
	"maxItems": true, "uniqueItems": true, "minProperties": true, "maxProperties": true,
}

// This graph counts possible schema traversal, not predicates or verdicts.
// A property/item edge consumes instance depth; a ref/composition edge does not.
// Only cycles of the latter are forbidden, preserving structural recursion.
func schemaWork(value any) ([]int, bool, error) {
	limits := ProfileLimits()
	var nodes []schemaNode
	paths := make(map[string]int)
	var references []schemaReference
	var visit func(any, string) (int, error)
	visit = func(value any, path string) (int, error) {
		index := len(nodes)
		nodes = append(nodes, schemaNode{local: 1})
		paths[path] = index
		if _, ok := value.(bool); ok {
			return index, nil
		}
		object, ok := value.(map[string]any)
		if !ok {
			return 0, errors.New("portable: schema must be an object or boolean")
		}
		for key, field := range object {
			childPath := path + "/" + pointerToken(key)
			switch key {
			case "$ref":
				ref, ok := field.(string)
				if !ok || ref != "#" && !strings.HasPrefix(ref, "#/") {
					return 0, errors.New("portable: schema references must be local JSON pointers")
				}
				decoded, err := url.PathUnescape(ref[1:])
				if err != nil {
					return 0, fmt.Errorf("portable: schema reference: %w", err)
				}
				references = append(references, schemaReference{source: index, path: decoded})
				if len(references) > limits.MaxReferences {
					return 0, limit("schema references", limits.MaxReferences)
				}
			case "$defs", "definitions", "properties", "patternProperties":
				children, ok := field.(map[string]any)
				if !ok {
					return 0, errors.New("portable: schema map must be an object")
				}
				for name, child := range children {
					if key == "patternProperties" {
						if err := checkPattern(name); err != nil {
							return 0, err
						}
					}
					childIndex, err := visit(child, childPath+"/"+pointerToken(name))
					if err != nil {
						return 0, err
					}
					if key == "properties" || key == "patternProperties" {
						nodes[index].descend = append(nodes[index].descend, childIndex)
					}
				}
			case "additionalProperties", "items", "contains", "propertyNames", "not", "if", "then", "else":
				child, err := visit(field, childPath)
				if err != nil {
					return 0, err
				}
				if key == "not" || key == "if" || key == "then" || key == "else" {
					nodes[index].same = append(nodes[index].same, child)
				} else {
					nodes[index].descend = append(nodes[index].descend, child)
				}
			case "allOf", "anyOf", "oneOf":
				children, ok := field.([]any)
				if !ok || len(children) == 0 {
					return 0, errors.New("portable: schema composition requires a nonempty array")
				}
				for n, child := range children {
					childIndex, err := visit(child, childPath+"/"+strconv.Itoa(n))
					if err != nil {
						return 0, err
					}
					nodes[index].same = append(nodes[index].same, childIndex)
				}
			case "$schema":
				if field != "https://json-schema.org/draft/2020-12/schema" {
					return 0, errors.New("portable: unsupported schema dialect")
				}
			case "pattern":
				pattern, ok := field.(string)
				if !ok {
					return 0, errors.New("portable: schema pattern must be a string")
				}
				if err := checkPattern(pattern); err != nil {
					return 0, err
				}
			case "enum":
				values, ok := field.([]any)
				if !ok || len(values) == 0 {
					return 0, errors.New("portable: enum requires a nonempty array")
				}
				weight := equalityWeight(field) - 1
				if equalityCost(0, len(values)-1, weight) > limits.MaxEqualityWork {
					return 0, limit("schema equality work", limits.MaxEqualityWork)
				}
				nodes[index].local += int(weight)
			case "const":
				nodes[index].local += int(equalityWeight(field))
			case "uniqueItems":
				nodes[index].unique, _ = field.(bool)
			default:
				if !schemaAnnotations[key] {
					return 0, fmt.Errorf("portable: unsupported schema keyword %q", key)
				}
			}
		}
		return index, nil
	}
	if _, err := visit(value, ""); err != nil {
		return nil, false, err
	}
	for _, ref := range references {
		target, ok := paths[ref.path]
		if !ok {
			return nil, false, errors.New("portable: unresolved schema reference")
		}
		nodes[ref.source].same = append(nodes[ref.source].same, target)
	}

	state := make([]byte, len(nodes))
	expansion := make([]int, len(nodes))
	depths := make([]int, len(nodes))
	var order []int
	var expand func(int) error
	expand = func(index int) error {
		if state[index] == 1 {
			return errors.New("portable: schema reference or composition cycle does not consume instance depth")
		}
		if state[index] == 2 {
			return nil
		}
		state[index] = 1
		expansion[index], depths[index] = 1, 1
		for _, child := range nodes[index].same {
			if err := expand(child); err != nil {
				return err
			}
			expansion[index] += expansion[child]
			depths[index] = max(depths[index], 1+depths[child])
			if expansion[index] > limits.MaxReferenceWork {
				return limit("same-value schema expansion", limits.MaxReferenceWork)
			}
			if depths[index] > limits.MaxReferenceDepth {
				return limit("same-value schema depth", limits.MaxReferenceDepth)
			}
		}
		state[index] = 2
		order = append(order, index)
		return nil
	}
	for index := range nodes {
		if err := expand(index); err != nil {
			return nil, false, err
		}
	}

	// Saturating dynamic programming bounds recursion without unfolding it.
	// Summing every possible child and multiplying by instance nodes is deliberately
	// conservative; admission never executes schema predicates to guess a cheaper path.
	work := make([]int, limits.MaxJSONDepth+1)
	previous := make([]int, len(nodes))
	for depth := range work {
		current := make([]int, len(nodes))
		for _, index := range order {
			cost := nodes[index].local
			for _, child := range nodes[index].same {
				cost = min(limits.MaxValidationWork+1, cost+current[child])
			}
			if depth > 0 {
				for _, child := range nodes[index].descend {
					cost = min(limits.MaxValidationWork+1, cost+previous[child])
				}
			}
			current[index] = cost
		}
		work[depth] = current[0]
		previous = current
	}
	unique := false
	visited := make([]bool, len(nodes))
	queue := []int{0}
	for len(queue) > 0 {
		index := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if visited[index] {
			continue
		}
		visited[index] = true
		unique = unique || nodes[index].unique
		queue = append(queue, nodes[index].same...)
		queue = append(queue, nodes[index].descend...)
	}
	return work, unique, nil
}

func equalityWeight(value any) uint64 {
	weight := uint64(1)
	switch value := value.(type) {
	case string:
		weight += uint64(len(value))
	case json.Number:
		weight += uint64(len(value))
	case []any:
		for _, child := range value {
			weight += equalityWeight(child)
		}
	case map[string]any:
		for key, child := range value {
			weight += uint64(max(1, len(key))) + equalityWeight(child)
		}
	}
	return weight
}

func pointerToken(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

// Fixed-width atoms and fixed repetitions cover identifiers without exposing
// Node's backtracking engine to author-supplied alternatives or variable repeats.
func checkPattern(pattern string) error {
	limits := ProfileLimits()
	if len(pattern) > limits.MaxPatternBytes {
		return limit("pattern bytes", limits.MaxPatternBytes)
	}
	inClass := false
	for index := 0; index < len(pattern); index++ {
		char := pattern[index]
		if char == '\\' {
			index++
			if index == len(pattern) || strings.ContainsRune("0123456789kKpP", rune(pattern[index])) {
				return errors.New("portable: unsupported pattern escape")
			}
			continue
		}
		if char == '[' {
			inClass = true
			continue
		}
		if char == ']' {
			inClass = false
			continue
		}
		if inClass {
			continue
		}
		if strings.ContainsRune("*+?|()", rune(char)) {
			return errors.New("portable: patterns require fixed-width atoms and repetitions")
		}
		if char == '{' {
			end := strings.IndexByte(pattern[index+1:], '}')
			if end == -1 {
				return errors.New("portable: pattern repetition is incomplete")
			}
			raw := pattern[index+1 : index+1+end]
			count, err := strconv.Atoi(raw)
			if err != nil || strings.ContainsFunc(raw, func(r rune) bool { return r < '0' || r > '9' }) || count < 0 || count > limits.MaxPatternRepeat {
				return errors.New("portable: pattern requires a bounded fixed repetition")
			}
			index += end + 1
		}
	}
	return nil
}
