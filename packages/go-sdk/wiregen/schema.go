package wiregen

const RefPrefix = "#/$defs/"

type SchemaType string

const (
	SchemaTypeArray   SchemaType = "array"
	SchemaTypeBoolean SchemaType = "boolean"
	SchemaTypeInteger SchemaType = "integer"
	SchemaTypeNull    SchemaType = "null"
	SchemaTypeNumber  SchemaType = "number"
	SchemaTypeObject  SchemaType = "object"
	SchemaTypeString  SchemaType = "string"
)

// Property values are any because a forbidden field is the boolean schema false.
type Schema struct {
	Ref              string         `json:"$ref,omitempty"`
	Type             SchemaType     `json:"type,omitempty"`
	Format           string         `json:"format,omitempty"`
	ContentEncoding  string         `json:"contentEncoding,omitempty"`
	Enum             []string       `json:"enum,omitempty"`
	MinLength        *int           `json:"minLength,omitzero"`
	MaxLength        *int           `json:"maxLength,omitzero"`
	Const            string         `json:"const,omitempty"`
	Pattern          string         `json:"pattern,omitempty"`
	TypeScriptType   string         `json:"-"`
	Minimum          *int64         `json:"minimum,omitzero"`
	ExclusiveMinimum *int64         `json:"exclusiveMinimum,omitzero"`
	Maximum          *int64         `json:"maximum,omitzero"`
	MinItems         *int           `json:"minItems,omitzero"`
	MaxItems         *int           `json:"maxItems,omitzero"`
	MinProperties    *int           `json:"minProperties,omitzero"`
	UniqueItems      bool           `json:"uniqueItems,omitzero"`
	Items            *Schema        `json:"items,omitzero"`
	Properties       map[string]any `json:"properties,omitempty"`
	AdditionalProps  any            `json:"additionalProperties,omitzero"`
	PropertyNames    *Schema        `json:"propertyNames,omitzero"`
	UnevaluatedProps *bool          `json:"unevaluatedProperties,omitzero"`
	Required         []string       `json:"required,omitempty"`
	OneOf            []*Schema      `json:"oneOf,omitempty"`
	AnyOf            []*Schema      `json:"anyOf,omitempty"`
	AllOf            []*Schema      `json:"allOf,omitempty"`
	If               *Schema        `json:"if,omitzero"`
	Then             *Schema        `json:"then,omitzero"`
}

const SchemaDialect = "https://json-schema.org/draft/2020-12/schema"

type Definitions map[string]*Schema
