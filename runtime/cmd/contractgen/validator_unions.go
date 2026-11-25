package main

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

func unionRequiresField(union dispatch.UnionSpec, field string) bool {
	if union.GoType == nil {
		return false
	}
	for _, variant := range union.Variants {
		if slices.Contains(variant.Required, field) {
			return true
		}
	}
	if pattern := union.PatternVariant; pattern != nil {
		return slices.Contains(pattern.Required, field)
	}
	return false
}

func enumChecks(shape reflect.Type) []string {
	recv := receiverName(shape)
	var checks []string
	for _, field := range protocol.WireFields(shape) {
		if values, found := enumValues(field.Type); found {
			checks = append(checks, fmt.Sprintf("closedEnum(%s, string(%s.%s), %s, %t)",
				strconv.Quote(field.Name), recv, field.GoName, valueList(values), field.Optional))
			continue
		}
		if field.Type.Kind() == reflect.Slice {
			if values, found := enumValues(field.Type.Elem()); found {
				checks = append(checks, fmt.Sprintf("closedEnumItems(%s, %s.%s, %s)",
					strconv.Quote(field.Name), recv, field.GoName, valueList(values)))
			}
		}
	}
	return checks
}

func unionChecks(union dispatch.UnionSpec, recv string) []string {
	if union.GoType == nil {
		return nil
	}
	paths := unionPaths(union)
	var checks []string
	if pattern := union.PatternVariant; pattern != nil {
		checks = append(checks, patternUnionTagCheck(union, *pattern, recv))
	}
	for _, variant := range union.Variants {
		checks = append(checks, literalUnionChecks(variant, paths, recv)...)
	}
	if pattern := union.PatternVariant; pattern != nil {
		checks = append(checks, patternUnionChecks(*pattern, paths, recv)...)
	}
	return checks
}

func patternUnionTagCheck(union dispatch.UnionSpec, pattern dispatch.PatternVariantSpec, recv string) string {
	field, ok := protocol.WireFieldNamed(union.GoType, dispatch.UnionDiscriminator)
	if !ok {
		panic(fmt.Sprintf("contractgen: %s has no discriminator %q", union.GoType.Name(), dispatch.UnionDiscriminator))
	}
	tags := make([]string, 0, len(union.Variants))
	for _, variant := range union.Variants {
		tags = append(tags, variant.Tag)
	}
	return fmt.Sprintf(
		"unionTag(%q, string(%s.%s), %s, %q)",
		dispatch.UnionDiscriminator, recv, field.GoName, valueList(tags), pattern.TagPattern,
	)
}

func literalUnionChecks(
	variant dispatch.VariantSpec,
	paths []string,
	recv string,
) []string {
	applies := fmt.Sprintf("wireFieldEquals(%s, %q, %q)", recv, dispatch.UnionDiscriminator, variant.Tag)
	checks := unionBranchChecks(applies, variant.Required, variant.Optional, paths, recv)
	return append(checks, allowedValueChecks(applies, variant.AllowedValues, recv)...)
}

func patternUnionChecks(
	pattern dispatch.PatternVariantSpec,
	paths []string,
	recv string,
) []string {
	applies := fmt.Sprintf("wireFieldMatches(%s, %q, %q)", recv, dispatch.UnionDiscriminator, pattern.TagPattern)
	return unionBranchChecks(applies, pattern.Required, pattern.Optional, paths, recv)
}

func unionBranchChecks(applies string, required, optional, paths []string, recv string) []string {
	allowed := append(slices.Clone(required), optional...)
	checks := make([]string, 0, len(required)+len(paths))
	for _, field := range required {
		checks = append(checks, fmt.Sprintf("requiredWhen(%s, %q, %s)", applies, field, recv))
	}
	for _, field := range paths {
		if slices.Contains(allowed, field) {
			continue
		}
		checks = append(checks, fmt.Sprintf("forbiddenWhen(%s, %q, %s)", applies, field, recv))
	}
	return checks
}

func unionPaths(union dispatch.UnionSpec) []string {
	var paths []string
	for _, variant := range union.Variants {
		for _, field := range append(slices.Clone(variant.Required), variant.Optional...) {
			if field != dispatch.UnionDiscriminator && !slices.Contains(paths, field) {
				paths = append(paths, field)
			}
		}
	}
	if pattern := union.PatternVariant; pattern != nil {
		for _, field := range append(slices.Clone(pattern.Required), pattern.Optional...) {
			if field != dispatch.UnionDiscriminator && !slices.Contains(paths, field) {
				paths = append(paths, field)
			}
		}
	}
	return paths
}
