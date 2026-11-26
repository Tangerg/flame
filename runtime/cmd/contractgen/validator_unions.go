package main

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
)

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
		checks = append(checks, literalUnionChecks(variant, paths)...)
	}
	if pattern := union.PatternVariant; pattern != nil {
		checks = append(checks, patternUnionChecks(*pattern, paths)...)
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
) []string {
	applies := fmt.Sprintf("%s.equals(%q, %q)", wireObjectParameter, dispatch.UnionDiscriminator, variant.Tag)
	checks := unionBranchChecks(applies, variant.Required, variant.Optional, paths)
	return append(checks, allowedValueChecks(applies, variant.AllowedValues)...)
}

func patternUnionChecks(
	pattern dispatch.PatternVariantSpec,
	paths []string,
) []string {
	applies := fmt.Sprintf("%s.matches(%q, %q)", wireObjectParameter, dispatch.UnionDiscriminator, pattern.TagPattern)
	return unionBranchChecks(applies, pattern.Required, pattern.Optional, paths)
}

func unionBranchChecks(applies string, required, optional, paths []string) []string {
	allowed := append(slices.Clone(required), optional...)
	checks := make([]string, 0, len(required)+len(paths))
	for _, field := range required {
		checks = append(checks, fmt.Sprintf("requiredWhen(%s, %q, %s)", applies, field, wireObjectParameter))
	}
	for _, field := range paths {
		if slices.Contains(allowed, field) {
			continue
		}
		checks = append(checks, fmt.Sprintf("forbiddenWhen(%s, %q, %s)", applies, field, wireObjectParameter))
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
