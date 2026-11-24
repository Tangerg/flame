import {
  ElicitRequestParamsSchema,
  ElicitResultSchema,
  BooleanSchemaSchema,
  NumberSchemaSchema,
  StringSchemaSchema,
  LegacyTitledEnumSchemaSchema,
  TitledSingleSelectEnumSchemaSchema,
  UntitledMultiSelectEnumSchemaSchema,
  TitledMultiSelectEnumSchemaSchema,
  type ElicitRequestFormParams,
  type ElicitRequestParams,
  type ElicitResult,
  type PrimitiveSchemaDefinition,
} from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import { parseJSON } from "@frame/sdk/json";
import { LIMITS } from "./limits";

export class InvalidPermission extends Error {}

export const PERMISSION_PERSISTENCE = ["session", "always"] as const;
export type PermissionPersistence = (typeof PERMISSION_PERSISTENCE)[number];

export interface NativePermission {
  readonly app: string;
  readonly riskLevel?: string;
  readonly subtitle?: string;
  readonly persist: readonly PermissionPersistence[];
}

export interface FieldChoice {
  readonly value: string;
  readonly label: string;
}

function object(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
function freeze<T>(value: T): T {
  if (value && typeof value === "object") {
    Object.values(value).forEach(freeze);
    Object.freeze(value);
  }
  return value;
}
function boundedJSON(raw: string): unknown {
  if (new TextEncoder().encode(raw).length > LIMITS.maxApprovalBytes)
    throw new InvalidPermission("computer: permission data exceeds its bound");
  try {
    return parseJSON(raw);
  } catch (cause) {
    throw new InvalidPermission(
      "computer: permission data must be valid JSON",
      { cause },
    );
  }
}
function knownFields(
  value: Record<string, unknown>,
  fields: readonly string[],
): void {
  if (Object.keys(value).some((key) => !fields.includes(key)))
    throw new InvalidPermission("computer: unsupported permission field");
}

const propertyDefinition = z.union([
  BooleanSchemaSchema.strict(),
  NumberSchemaSchema.strict(),
  StringSchemaSchema.extend({
    enum: LegacyTitledEnumSchemaSchema.shape.enum.optional(),
    enumNames: LegacyTitledEnumSchemaSchema.shape.enumNames,
    oneOf: z
      .array(TitledSingleSelectEnumSchemaSchema.shape.oneOf.element.strict())
      .optional(),
  }).strict(),
  UntitledMultiSelectEnumSchemaSchema.extend({
    items: UntitledMultiSelectEnumSchemaSchema.shape.items.strict(),
  }).strict(),
  TitledMultiSelectEnumSchemaSchema.extend({
    items: TitledMultiSelectEnumSchemaSchema.shape.items
      .extend({
        anyOf: z.array(
          TitledMultiSelectEnumSchemaSchema.shape.items.shape.anyOf.element.strict(),
        ),
      })
      .strict(),
  }).strict(),
]);

function bounds(
  minimum: number | undefined,
  maximum: number | undefined,
  kind: "number" | "count" = "number",
): void {
  if (
    (kind === "count" &&
      [minimum, maximum].some(
        (value) =>
          value !== undefined && (!Number.isSafeInteger(value) || value < 0),
      )) ||
    (minimum !== undefined && maximum !== undefined && minimum > maximum)
  )
    throw new InvalidPermission(
      "computer: permission constraints are inconsistent",
    );
}

type FieldDefinition<Type extends PrimitiveSchemaDefinition["type"]> = Extract<
  PrimitiveSchemaDefinition,
  { type: Type }
>;

function fieldSchema(field: PrimitiveSchemaDefinition): z.ZodType {
  const admitted = propertyDefinition.safeParse(field);
  if (!admitted.success)
    throw new InvalidPermission(
      "computer: unsupported permission field definition",
      { cause: admitted.error },
    );
  const result = typedFieldSchema(field);
  if (field.default !== undefined && !result.safeParse(field.default).success)
    throw new InvalidPermission(
      "computer: permission default does not satisfy its field",
    );
  return result;
}

function typedFieldSchema(field: PrimitiveSchemaDefinition): z.ZodType {
  switch (field.type) {
    case "boolean":
      return z.boolean();
    case "integer":
    case "number":
      return numberSchema(field);
    case "string":
      return stringSchema(field);
    case "array":
      return arraySchema(field);
  }
}

function numberSchema(field: FieldDefinition<"integer" | "number">): z.ZodType {
  bounds(field.minimum, field.maximum);
  if (
    field.type === "integer" &&
    field.minimum !== undefined &&
    field.maximum !== undefined &&
    Math.ceil(field.minimum) > Math.floor(field.maximum)
  )
    throw new InvalidPermission(
      "computer: permission constraints contain no integer",
    );
  let schema = field.type === "integer" ? z.number().int() : z.number();
  if (field.minimum !== undefined) schema = schema.min(field.minimum);
  if (field.maximum !== undefined) schema = schema.max(field.maximum);
  return schema;
}

function stringChoices(
  field: FieldDefinition<"string">,
): FieldChoice[] | undefined {
  if ("oneOf" in field)
    return field.oneOf.map((item) => ({
      value: item.const,
      label: item.title,
    }));
  if ("enum" in field)
    return field.enum.map((value, index) => ({
      value,
      label:
        ("enumNames" in field ? field.enumNames?.[index] : undefined) ?? value,
    }));
  return undefined;
}

function arrayChoices(field: FieldDefinition<"array">): FieldChoice[] {
  return "anyOf" in field.items
    ? field.items.anyOf.map((item) => ({
        value: item.const,
        label: item.title,
      }))
    : field.items.enum.map((value) => ({ value, label: value }));
}

// MCP spells a closed choice set as enum, enum with enumNames, oneOf or anyOf;
// validation and the console read it through this one projection.
export function fieldChoices(
  field: PrimitiveSchemaDefinition,
): FieldChoice[] | undefined {
  if (field.type === "string") return stringChoices(field);
  if (field.type === "array") return arrayChoices(field);
  return undefined;
}

function distinctValues(choices: FieldChoice[]): string[] {
  const values = choices.map((choice) => choice.value);
  if (!values.length || new Set(values).size !== values.length)
    throw new InvalidPermission(
      "computer: permission choices must be distinct",
    );
  return values;
}

function stringChoiceValues(
  field: FieldDefinition<"string">,
): string[] | undefined {
  if ("enum" in field && "oneOf" in field)
    throw new InvalidPermission(
      "computer: permission choices require one representation",
    );
  if (
    "enumNames" in field &&
    field.enumNames !== undefined &&
    (!("enum" in field) || field.enumNames.length !== field.enum.length)
  )
    throw new InvalidPermission(
      "computer: permission choice labels must match their values",
    );
  const choices = stringChoices(field);
  return choices && distinctValues(choices);
}

function stringSchema(field: FieldDefinition<"string">): z.ZodType {
  const choices = stringChoiceValues(field);
  bounds(
    "minLength" in field ? field.minLength : undefined,
    "maxLength" in field ? field.maxLength : undefined,
    "count",
  );
  let schema = z.string();
  if ("minLength" in field && field.minLength !== undefined)
    schema = schema.min(field.minLength);
  if ("maxLength" in field && field.maxLength !== undefined)
    schema = schema.max(field.maxLength);
  let result: z.ZodType = choices
    ? schema.refine((value) => choices.includes(value))
    : schema;
  if ("format" in field && field.format) {
    const formats = {
      email: z.email(),
      uri: z.url(),
      date: z.iso.date(),
      "date-time": z.iso.datetime({ offset: true }),
    };
    result = result.and(formats[field.format]);
  }
  if (choices && !choices.some((value) => result.safeParse(value).success))
    throw new InvalidPermission(
      "computer: permission constraints exclude every choice",
    );
  return result;
}

function arraySchema(field: FieldDefinition<"array">): z.ZodType {
  bounds(field.minItems, field.maxItems, "count");
  const choices = distinctValues(arrayChoices(field));
  if (field.minItems !== undefined && field.minItems > choices.length)
    throw new InvalidPermission(
      "computer: permission requires more distinct selections than it offers",
    );
  let schema = z.array(z.string().refine((value) => choices.includes(value)));
  if (field.minItems !== undefined) schema = schema.min(field.minItems);
  if (field.maxItems !== undefined) schema = schema.max(field.maxItems);
  return schema.refine((value) => new Set(value).size === value.length);
}

function admittedRequest(raw: unknown): ElicitRequestParams {
  const parsed = ElicitRequestParamsSchema.safeParse(raw);
  if (!parsed.success || !object(raw))
    throw new InvalidPermission("computer: unsupported permission request", {
      cause: parsed.error,
    });
  const request = raw as ElicitRequestParams;
  knownFields(
    raw,
    request.mode === "url"
      ? ["mode", "message", "url", "elicitationId", "_meta"]
      : ["mode", "message", "requestedSchema", "_meta"],
  );
  return request;
}

function assertOpenableURL(raw: string): void {
  const url = new URL(raw);
  if (
    !["https:", "http:"].includes(url.protocol) ||
    url.username ||
    url.password
  )
    throw new InvalidPermission(
      "computer: permission URL must be HTTP or HTTPS without credentials",
    );
}

function formContent(
  requestedSchema: ElicitRequestFormParams["requestedSchema"],
): z.ZodType {
  knownFields(requestedSchema, ["type", "properties", "required"]);
  const properties = Object.entries(requestedSchema.properties);
  const required = new Set(requestedSchema.required ?? []);
  if (
    properties.length > LIMITS.maxPermissionFields ||
    required.size !== (requestedSchema.required?.length ?? 0) ||
    [...required].some(
      (name) => !Object.hasOwn(requestedSchema.properties, name),
    )
  )
    throw new InvalidPermission("computer: invalid permission form fields");
  return z.strictObject(
    Object.fromEntries(
      properties.map(([name, field]) => {
        const schema = fieldSchema(field);
        return [name, required.has(name) ? schema : schema.optional()];
      }),
    ),
  );
}

// Codex marks approvals for its own computer-use tool calls with this metadata;
// only such an empty form offers persistence scopes.
function nativePermission(
  request: ElicitRequestFormParams,
): NativePermission | undefined {
  const meta = request._meta;
  const tool = meta?.tool_params;
  if (
    Object.keys(request.requestedSchema.properties).length ||
    meta?.codex_approval_kind !== "mcp_tool_call" ||
    meta.connector_id !== "computer-use" ||
    !object(tool) ||
    typeof tool.app !== "string" ||
    !tool.app
  )
    return undefined;
  const offered = new Set(Array.isArray(meta.persist) ? meta.persist : []);
  const subtitle = meta.warningSubtitle ?? meta.subtitle;
  return freeze({
    app: tool.app,
    ...(typeof meta.riskLevel === "string"
      ? { riskLevel: meta.riskLevel }
      : {}),
    ...(typeof subtitle === "string" ? { subtitle } : {}),
    persist: PERMISSION_PERSISTENCE.filter((scope) => offered.has(scope)),
  });
}

export class PermissionRequest {
  readonly request: ElicitRequestParams;
  readonly native?: NativePermission;
  readonly #content?: z.ZodType;

  constructor(requestJSON: string) {
    const request = admittedRequest(boundedJSON(requestJSON));
    if (request.mode === "url") assertOpenableURL(request.url);
    else {
      this.#content = formContent(request.requestedSchema);
      this.native = nativePermission(request);
    }
    this.request = freeze(request);
    Object.freeze(this);
  }

  reply(responseJSON: string): ElicitResult {
    const value = boundedJSON(responseJSON);
    if (!object(value) || !ElicitResultSchema.safeParse(value).success)
      throw new InvalidPermission("computer: invalid permission response");
    knownFields(value, ["action", "content", "_meta"]);
    if (value.action !== "accept") {
      if (value.content !== undefined || value._meta !== undefined)
        throw new InvalidPermission(
          "computer: declined permission cannot submit content or persistence",
        );
    } else if (this.request.mode === "url") {
      if (value.content !== undefined || value._meta !== undefined)
        throw new InvalidPermission(
          "computer: URL permission cannot submit form content",
        );
    } else {
      if (!this.#content!.safeParse(value.content).success)
        throw new InvalidPermission(
          "computer: permission response does not satisfy the requested form",
        );
      if (value._meta !== undefined) {
        if (!object(value._meta))
          throw new InvalidPermission(
            "computer: invalid permission persistence",
          );
        knownFields(value._meta, ["persist"]);
        if (
          !this.native ||
          !this.native.persist.includes(
            value._meta.persist as PermissionPersistence,
          )
        )
          throw new InvalidPermission(
            "computer: permission persistence was not offered",
          );
      }
    }
    return freeze(value as ElicitResult);
  }
}
