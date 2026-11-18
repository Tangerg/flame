package dispatch

import (
	"reflect"

	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type allowedValueBranchPayload struct {
	State string `json:"state"`
}

type allowedValueBranchFixture struct {
	Type  string                     `json:"type"`
	Left  *allowedValueBranchPayload `json:"left,omitempty"`
	Right *allowedValueBranchPayload `json:"right,omitempty"`
}

func TestNotificationValidationRejectsUnpublishableParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec NotificationSpec
		want string
	}{
		{name: "empty method", spec: NotificationSpec{}, want: "method name"},
		{
			name: "missing params",
			spec: NotificationSpec{Name: "notifications.test.event"},
			want: "no params type",
		},
		{
			name: "anonymous params",
			spec: NotificationSpec{
				Name: "notifications.test.event", ParamsType: reflect.TypeFor[struct{}](),
			},
			want: "named struct",
		},
		{
			name: "wrong namespace",
			spec: NotificationSpec{
				Name: "runtime.event", ParamsType: reflect.TypeFor[protocol.RuntimeEventNotification](),
			},
			want: "notifications namespace",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.spec.validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error = %v, want it to mention %q", err, test.want)
			}
		})
	}
}

// TestUnionValidationCatchesTheDriftItExistsFor covers the three ways a spec goes
// stale. The third is the one that actually happens: a field is added to the
// struct and no variant claims it, so a generated schema would permit it under
// every tag.
func TestUnionValidationCatchesTheDriftItExistsFor(t *testing.T) {
	t.Parallel()

	validContentBlockVariants := []VariantSpec{
		{Tag: "text", Required: []string{"text"}},
		{Tag: "image", Required: []string{"mime", "data"}},
	}
	tests := []struct {
		name string
		spec UnionSpec
		want string
	}{{
		name: "a variant names a field the struct does not have",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants: []VariantSpec{
				{Tag: "text", Required: []string{"text"}},
				{Tag: "image", Required: []string{"mime", "data", "altText"}},
			},
		},
		want: `no JSON field "altText"`,
	}, {
		name: "a struct field belongs to no variant",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants: []VariantSpec{
				{Tag: "text", Required: []string{"text"}},
				{Tag: "image", Required: []string{"mime"}},
			},
		},
		want: `field "data" belongs to no variant`,
	}, {
		name: "a discriminator other than type",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "kind",
			Variants: []VariantSpec{{Tag: "text", Required: []string{"text"}}},
		},
		want: `discriminator is "kind"`,
	}, {
		name: "a duplicated tag",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants: []VariantSpec{
				{Tag: "text", Required: []string{"text", "mime", "data"}},
				{Tag: "text", Required: []string{"text"}},
			},
		},
		want: `variant "text" is declared twice`,
	}, {
		name: "allowed values belong to another variant",
		spec: UnionSpec{
			GoType: reflect.TypeFor[allowedValueBranchFixture](), Discriminator: "type",
			Variants: []VariantSpec{
				{
					Tag: "left", Required: []string{"left"},
					AllowedValues: []AllowedValueSet{{Field: "right.state", Values: []string{"ready"}}},
				},
				{Tag: "right", Required: []string{"right"}},
			},
		},
		want: `allowed-values field "right.state" is not claimed by the variant`,
	}, {
		name: "an empty forbidden field",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants:  validContentBlockVariants,
			Forbidden: []string{""},
		},
		want: `forbidden field 0 has no name`,
	}, {
		name: "a nested forbidden field",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants:  validContentBlockVariants,
			Forbidden: []string{"legacy.value"},
		},
		want: `forbidden field "legacy.value" must be a top-level JSON member`,
	}, {
		name: "a duplicated forbidden field",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants:  validContentBlockVariants,
			Forbidden: []string{"legacy", "legacy"},
		},
		want: `forbidden field "legacy" is declared twice`,
	}, {
		name: "a forbidden field still on the wire shape",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants:  validContentBlockVariants,
			Forbidden: []string{"data"},
		},
		want: `forbidden field "data" still exists on the Go wire shape`,
	}, {
		name: "an invalid extension pattern",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants: validContentBlockVariants,
			PatternVariant: &PatternVariantSpec{
				TagPattern: "[", TypeScriptType: "`plugin:${string}/${string}`",
			},
		},
		want: "invalid pattern variant tag",
	}, {
		name: "an extension pattern overlaps a literal tag",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants: validContentBlockVariants,
			PatternVariant: &PatternVariantSpec{
				TagPattern: "^text$", TypeScriptType: "`plugin:${string}/${string}`",
			},
		},
		want: `pattern variant also matches literal tag "text"`,
	}, {
		name: "an extension pattern needs a TypeScript type",
		spec: UnionSpec{
			GoType: reflect.TypeFor[protocol.ContentBlock](), Discriminator: "type",
			Variants:       validContentBlockVariants,
			PatternVariant: &PatternVariantSpec{TagPattern: "^plugin:"},
		},
		want: "pattern variant needs a TypeScript type",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.spec.validate()
			if err == nil {
				t.Fatal("validate accepted a spec that does not match its struct")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestUnionValidationAcceptsRemovedForbiddenField(t *testing.T) {
	t.Parallel()

	spec := UnionSpec{
		GoType:        reflect.TypeFor[protocol.ContentBlock](),
		Discriminator: "type",
		Variants: []VariantSpec{
			{Tag: "text", Required: []string{"text"}},
			{Tag: "image", Required: []string{"mime", "data"}},
		},
		Forbidden: []string{"legacy"},
	}
	if err := spec.validate(); err != nil {
		t.Fatalf("validate rejected a protocol-level negative invariant: %v", err)
	}
}
