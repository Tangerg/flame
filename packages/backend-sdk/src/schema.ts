import { Decimal } from "decimal.js";
import { Validator, type CustomProperty, type Schema } from "jsonschema";
import { fullFormats } from "ajv-formats/dist/formats";
import {
  parseJSON,
  stringifyJSON,
  isJSONNumber as numeric,
  equalJSON as equal,
  isJSONInteger,
  type JSONNumber,
} from "@flame/runtime-client";

type RecordValue = Record<string, unknown>;
const decimal = (value: JSONNumber) => new Decimal(value.toString());

const maps = new Set([
  "$defs",
  "definitions",
  "properties",
  "patternProperties",
]);
const singles = new Set([
  "additionalProperties",
  "items",
  "contains",
  "propertyNames",
  "not",
  "if",
  "then",
  "else",
]);
const arrays = new Set(["allOf", "anyOf", "oneOf"]);
const annotations = new Set([
  "$schema",
  "$id",
  "$comment",
  "$defs",
  "definitions",
  "title",
  "description",
  "default",
  "examples",
  "readOnly",
  "writeOnly",
  "deprecated",
  "contentEncoding",
  "contentMediaType",
]);
const supported = new Set([
  ...annotations,
  ...maps,
  ...singles,
  ...arrays,
  "$ref",
  "type",
  "enum",
  "const",
  "required",
  "minimum",
  "maximum",
  "exclusiveMinimum",
  "exclusiveMaximum",
  "multipleOf",
  "minLength",
  "maxLength",
  "pattern",
  "format",
  "minItems",
  "maxItems",
  "uniqueItems",
  "minProperties",
  "maxProperties",
]);

// jsonschema supplies traversal and composition. Its supported instance-local
// hooks adapt numeric comparison to the public exact JSON representation.
export function compileSchema(source: unknown): (value: unknown) => void {
  const validator = new Validator() as Validator & {
    types: Record<string, (value: unknown) => boolean>;
  };
  validator.types.number = numeric;
  validator.types.integer = isJSONInteger;
  validator.types.object = (value) =>
    value !== null &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    !numeric(value);
  for (const [name, format] of Object.entries(fullFormats)) {
    if (
      typeof format === "object" &&
      "type" in format &&
      format.type === "number"
    )
      continue;
    const rule: unknown =
      typeof format === "object" && "validate" in format
        ? format.validate
        : format;
    validator.customFormats[name] = (value: unknown) => {
      if (typeof value !== "string") return true;
      if (rule instanceof RegExp) return rule.test(value);
      if (typeof rule === "string") return new RegExp(rule, "u").test(value);
      if (rule === true) return true;
      if (typeof rule === "function") {
        const verdict: unknown = rule(value);
        if (typeof verdict === "boolean") return verdict;
      }
      throw new Error("plugin: format must produce a synchronous verdict");
    };
  }
  validator.attributes.format = ((value: unknown, schema: Schema) => {
    const check = validator.customFormats[schema.format!];
    return typeof value !== "string" || !check || check(value)
      ? ""
      : "format constraint failed";
  }) as CustomProperty;
  for (const name of annotations) validator.attributes[name] = () => "";
  for (const [keyword, compare] of Object.entries({
    minimum: (left: Decimal, right: Decimal) => left.gte(right),
    maximum: (left: Decimal, right: Decimal) => left.lte(right),
    exclusiveMinimum: (left: Decimal, right: Decimal) => left.gt(right),
    exclusiveMaximum: (left: Decimal, right: Decimal) => left.lt(right),
    multipleOf: (left: Decimal, right: Decimal) => left.mod(right).isZero(),
  })) {
    validator.attributes[keyword] = ((value: unknown, schema: Schema) => {
      if (!numeric(value)) return "";
      const bound = (schema as RecordValue)[keyword];
      if (!numeric(bound))
        throw new Error("plugin: invalid numeric schema constraint");
      return compare(decimal(value), decimal(bound))
        ? ""
        : "numeric constraint failed";
    }) as CustomProperty;
  }
  validator.attributes.enum = ((value: unknown, schema: Schema) =>
    schema.enum!.some((candidate: unknown) => equal(value, candidate))
      ? ""
      : "enum constraint failed") as CustomProperty;
  validator.attributes.const = ((value: unknown, schema: Schema) =>
    equal(value, schema.const)
      ? ""
      : "const constraint failed") as CustomProperty;
  validator.attributes.uniqueItems = ((value: unknown, schema: Schema) => {
    if (!schema.uniqueItems || !Array.isArray(value)) return "";
    return value.some((item, index) =>
      value.slice(0, index).some((previous) => equal(item, previous)),
    )
      ? "unique items constraint failed"
      : "";
  }) as CustomProperty;

  function admit(value: unknown): unknown {
    if (typeof value === "boolean") return value;
    if (
      value === null ||
      typeof value !== "object" ||
      Array.isArray(value) ||
      numeric(value)
    )
      throw new Error("plugin: schema must be an object or boolean");
    const schema = value as RecordValue;
    for (const [key, field] of Object.entries(schema)) {
      if (!supported.has(key))
        throw new Error(`plugin: unsupported schema keyword ${key}`);
      if (maps.has(key)) {
        if (field === null || typeof field !== "object" || Array.isArray(field))
          throw new Error("plugin: schema map is invalid");
        for (const [name, child] of Object.entries(field))
          (field as RecordValue)[name] = admit(child);
      } else if (singles.has(key)) schema[key] = admit(field);
      else if (arrays.has(key)) {
        if (!Array.isArray(field) || field.length === 0)
          throw new Error("plugin: schema composition is invalid");
        schema[key] = field.map(admit);
      }
      if (
        [
          "minimum",
          "maximum",
          "exclusiveMinimum",
          "exclusiveMaximum",
          "multipleOf",
        ].includes(key) &&
        (!numeric(field) || (key === "multipleOf" && !decimal(field).gt(0)))
      )
        throw new Error("plugin: numeric schema constraint is invalid");
      if (
        [
          "minLength",
          "maxLength",
          "minItems",
          "maxItems",
          "minProperties",
          "maxProperties",
        ].includes(key) &&
        (typeof field !== "number" || !Number.isSafeInteger(field) || field < 0)
      )
        throw new Error("plugin: schema size constraint is invalid");
      if (
        key === "required" &&
        (!Array.isArray(field) ||
          field.some((name) => typeof name !== "string") ||
          new Set(field).size !== field.length)
      )
        throw new Error("plugin: schema required members are invalid");
      if (key === "enum" && (!Array.isArray(field) || field.length === 0))
        throw new Error("plugin: schema enum is invalid");
      if (
        key === "enum" &&
        (field as unknown[]).some((item, index, values) =>
          values.slice(0, index).some((previous) => equal(item, previous)),
        )
      )
        throw new Error("plugin: schema enum has duplicate values");
      if (key === "type") {
        const types = Array.isArray(field) ? field : [field];
        if (
          types.length === 0 ||
          types.some(
            (type) =>
              typeof type !== "string" ||
              ![
                "null",
                "boolean",
                "object",
                "array",
                "number",
                "integer",
                "string",
              ].includes(type),
          ) ||
          new Set(types).size !== types.length
        )
          throw new Error("plugin: schema type is invalid");
      }
      if (
        [
          "pattern",
          "format",
          "$id",
          "$schema",
          "$comment",
          "title",
          "description",
          "contentEncoding",
          "contentMediaType",
        ].includes(key) &&
        typeof field !== "string"
      )
        throw new Error("plugin: schema string constraint is invalid");
      if (
        ["uniqueItems", "readOnly", "writeOnly", "deprecated"].includes(key) &&
        typeof field !== "boolean"
      )
        throw new Error("plugin: schema boolean constraint is invalid");
      if (key === "examples" && !Array.isArray(field))
        throw new Error("plugin: schema examples must be an array");
      if (
        key === "$schema" &&
        field !== "https://json-schema.org/draft/2020-12/schema"
      )
        throw new Error("plugin: unsupported schema dialect");
      if (key === "$id")
        throw new Error("plugin: nested schema resources are unsupported");
      if (key === "pattern") new RegExp(field as string, "u");
    }
    if (schema.$ref !== undefined) {
      if (
        typeof schema.$ref !== "string" ||
        (schema.$ref !== "#" && !schema.$ref.startsWith("#/"))
      )
        throw new Error("plugin: schema references must be local");
      const ref = schema.$ref;
      delete schema.$ref;
      schema.allOf = [
        ...((schema.allOf as unknown[] | undefined) ?? []),
        { $ref: ref },
      ];
    }
    return schema;
  }

  const schema = admit(parseJSON(stringifyJSON(source))) as Schema;
  function checkReferences(value: unknown): void {
    if (!value || typeof value !== "object") return;
    if (Array.isArray(value)) {
      value.forEach(checkReferences);
      return;
    }
    const record = value as RecordValue;
    if (typeof record.$ref === "string") {
      const pointer = decodeURIComponent(record.$ref.slice(1));
      let target: unknown = schema;
      for (const segment of pointer === "" ? [] : pointer.slice(1).split("/")) {
        const key = segment.replaceAll("~1", "/").replaceAll("~0", "~");
        if (
          !target ||
          typeof target !== "object" ||
          !Object.hasOwn(target, key)
        )
          throw new Error("plugin: unresolved local schema reference");
        target = (target as RecordValue)[key];
      }
      if (
        typeof target !== "boolean" &&
        (target === null ||
          typeof target !== "object" ||
          Array.isArray(target) ||
          numeric(target))
      )
        throw new Error("plugin: schema reference must identify a schema");
    }
    for (const [name, field] of Object.entries(record)) {
      if (maps.has(name))
        Object.values(field as RecordValue).forEach(checkReferences);
      else if (singles.has(name) || arrays.has(name)) checkReferences(field);
    }
  }
  checkReferences(schema);
  const base = "https://flame.invalid/schema";
  validator.addSchema(schema, base);
  const validate = (value: unknown) => {
    stringifyJSON(value);
    if (
      !validator.validate(value, schema, {
        base,
        required: true,
        allowUnknownAttributes: false,
      }).valid
    )
      throw new Error("plugin: value does not satisfy its schema");
  };
  validator.validate(undefined, schema, {
    base,
    allowUnknownAttributes: false,
  });
  return validate;
}
