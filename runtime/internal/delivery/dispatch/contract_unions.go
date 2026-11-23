package dispatch

import (
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func (u UnionSpec) validate() error {
	if u.GoType == nil || u.GoType.Kind() != reflect.Struct {
		return fmt.Errorf("union spec needs a struct type, got %v", u.GoType)
	}
	validation := unionValidation{
		spec: u, name: u.GoType.Name(),
		accounted: []string{UnionDiscriminator},
		tags:      make(map[string]bool, len(u.Variants)),
	}
	if err := validation.validateDiscriminator(); err != nil {
		return err
	}
	if err := validation.validateForbiddenFields(); err != nil {
		return err
	}
	for _, variant := range u.Variants {
		if err := validation.validateLiteralVariant(variant); err != nil {
			return err
		}
	}
	if err := validation.validatePatternVariant(); err != nil {
		return err
	}
	return validation.validateCoverage()
}

type unionValidation struct {
	spec      UnionSpec
	name      string
	accounted []string
	tags      map[string]bool
}

func (u *unionValidation) validateDiscriminator() error {
	spec := u.spec
	if err := protocol.ValidateWirePath(spec.GoType, UnionDiscriminator); err != nil {
		return fmt.Errorf("%s: %w", u.name, err)
	}
	if len(spec.Variants) == 0 {
		return fmt.Errorf("%s: a union with no literal variants describes nothing", u.name)
	}
	return nil
}

func (u *unionValidation) validateForbiddenFields() error {
	spec := u.spec
	for index, field := range spec.Forbidden {
		switch {
		case field == "":
			return fmt.Errorf("%s: forbidden field %d has no name", u.name, index)
		case strings.Contains(field, "."):
			return fmt.Errorf(
				"%s: forbidden field %q must be a top-level JSON member",
				u.name,
				field,
			)
		case slices.Contains(spec.Forbidden[:index], field):
			return fmt.Errorf("%s: forbidden field %q is declared twice", u.name, field)
		case slices.Contains(protocol.WireFieldNames(spec.GoType), field):
			return fmt.Errorf(
				"%s: forbidden field %q still exists on the Go wire shape",
				u.name,
				field,
			)
		}
	}
	return nil
}

func (u *unionValidation) validateLiteralVariant(variant VariantSpec) error {
	if variant.Tag == "" {
		return fmt.Errorf("%s: a variant needs a tag", u.name)
	}
	if u.tags[variant.Tag] {
		return fmt.Errorf("%s: variant %q is declared twice", u.name, variant.Tag)
	}
	u.tags[variant.Tag] = true
	owner := fmt.Sprintf("%s variant %q", u.name, variant.Tag)
	if err := u.claimFields(
		owner,
		variant.Required,
		variant.Optional,
	); err != nil {
		return err
	}
	return validateVariantAllowedValueSets(owner, u.spec.GoType, variant)
}

func (u *unionValidation) validatePatternVariant() error {
	pattern := u.spec.PatternVariant
	if pattern == nil {
		return nil
	}
	compiled, err := regexp.Compile(pattern.TagPattern)
	switch {
	case pattern.TagPattern == "":
		return fmt.Errorf("%s: pattern variant needs a tag pattern", u.name)
	case err != nil:
		return fmt.Errorf(
			"%s: invalid pattern variant tag %q: %w",
			u.name,
			pattern.TagPattern,
			err,
		)
	case pattern.TypeScriptType == "":
		return fmt.Errorf("%s: pattern variant needs a TypeScript type", u.name)
	}
	for tag := range u.tags {
		if compiled.MatchString(tag) {
			return fmt.Errorf(
				"%s: pattern variant also matches literal tag %q",
				u.name,
				tag,
			)
		}
	}
	return u.claimFields(
		u.name+" pattern variant",
		pattern.Required,
		pattern.Optional,
	)
}

func (u *unionValidation) claimFields(
	owner string,
	required []string,
	optional []string,
) error {
	for index, field := range required {
		if slices.Contains(required[:index], field) {
			return fmt.Errorf("%s: required field %q is declared twice", owner, field)
		}
	}
	for index, field := range optional {
		switch {
		case slices.Contains(optional[:index], field):
			return fmt.Errorf("%s: optional field %q is declared twice", owner, field)
		case slices.Contains(required, field):
			return fmt.Errorf("%s: field %q cannot be both required and optional", owner, field)
		}
	}
	for _, field := range slices.Concat(required, optional) {
		if err := protocol.ValidateWirePath(u.spec.GoType, field); err != nil {
			return fmt.Errorf("%s: %w", owner, err)
		}
		root := strings.Split(field, ".")[0]
		if !slices.Contains(u.accounted, root) {
			u.accounted = append(u.accounted, root)
		}
	}
	return nil
}

func (u *unionValidation) validateCoverage() error {
	for _, field := range protocol.WireFieldNames(u.spec.GoType) {
		if !slices.Contains(u.accounted, field) {
			return fmt.Errorf(
				"%s: field %q belongs to no variant — every union field must name its tag",
				u.name,
				field,
			)
		}
	}
	return nil
}
