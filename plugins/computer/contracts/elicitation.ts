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
  type ElicitRequestParams,
  type ElicitResult,
  type PrimitiveSchemaDefinition,
} from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import { parseJSON } from "@qimu/sdk/json";
import { LIMITS } from "./limits";

export class InvalidPermission extends Error {}

export interface NativePermission {
  readonly app: string;
  readonly riskLevel?: string;
  readonly subtitle?: string;
  readonly persist: readonly ("session" | "always")[];
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
  count = false,
): void {
  if (
    (count &&
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

function fieldSchema(field: PrimitiveSchemaDefinition): z.ZodType {
  const admitted = propertyDefinition.safeParse(field);
  if (!admitted.success)
    throw new InvalidPermission(
      "computer: unsupported permission field definition",
      { cause: admitted.error },
    );
  let result: z.ZodType;
  switch (field.type) {
    case "boolean":
      result = z.boolean();
      break;
    case "integer":
    case "number": {
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
      result = schema;
      break;
    }
    case "string": {
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
      const choices =
        "enum" in field
          ? field.enum
          : "oneOf" in field
            ? field.oneOf.map((item) => item.const)
            : undefined;
      if (
        choices &&
        (!choices.length || new Set(choices).size !== choices.length)
      )
        throw new InvalidPermission(
          "computer: permission choices must be distinct",
        );
      bounds(
        "minLength" in field ? field.minLength : undefined,
        "maxLength" in field ? field.maxLength : undefined,
        true,
      );
      let schema = z.string();
      if ("minLength" in field && field.minLength !== undefined)
        schema = schema.min(field.minLength);
      if ("maxLength" in field && field.maxLength !== undefined)
        schema = schema.max(field.maxLength);
      result = choices
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
      break;
    }
    case "array": {
      bounds(field.minItems, field.maxItems, true);
      const choices =
        "enum" in field.items
          ? field.items.enum
          : field.items.anyOf.map((item) => item.const);
      if (!choices.length || new Set(choices).size !== choices.length)
        throw new InvalidPermission(
          "computer: permission choices must be distinct",
        );
      if (field.minItems !== undefined && field.minItems > choices.length)
        throw new InvalidPermission(
          "computer: permission requires more distinct selections than it offers",
        );
      let schema = z.array(
        z.string().refine((value) => choices.includes(value)),
      );
      if (field.minItems !== undefined) schema = schema.min(field.minItems);
      if (field.maxItems !== undefined) schema = schema.max(field.maxItems);
      result = schema.refine((value) => new Set(value).size === value.length);
      break;
    }
  }
  if (field.default !== undefined && !result.safeParse(field.default).success)
    throw new InvalidPermission(
      "computer: permission default does not satisfy its field",
    );
  return result;
}

export class PermissionRequest {
  readonly request: ElicitRequestParams;
  readonly native?: NativePermission;
  readonly #content?: z.ZodType;

  constructor(requestJSON: string) {
    const raw = boundedJSON(requestJSON);
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
    if (request.mode === "url") {
      const url = new URL(request.url);
      if (
        !["https:", "http:"].includes(url.protocol) ||
        url.username ||
        url.password
      )
        throw new InvalidPermission(
          "computer: permission URL must be HTTP or HTTPS without credentials",
        );
    } else {
      const { requestedSchema } = request;
      knownFields(requestedSchema, ["type", "properties", "required"]);
      const properties = Object.entries(requestedSchema.properties);
      const required = new Set(requestedSchema.required ?? []);
      if (
        properties.length > 64 ||
        required.size !== (requestedSchema.required?.length ?? 0) ||
        [...required].some(
          (name) => !Object.hasOwn(requestedSchema.properties, name),
        )
      )
        throw new InvalidPermission("computer: invalid permission form fields");
      this.#content = z.strictObject(
        Object.fromEntries(
          properties.map(([name, field]) => {
            const schema = fieldSchema(field);
            return [name, required.has(name) ? schema : schema.optional()];
          }),
        ),
      );
      const meta = request._meta;
      const tool = meta?.tool_params;
      if (
        !properties.length &&
        !required.size &&
        meta?.codex_approval_kind === "mcp_tool_call" &&
        meta.connector_id === "computer-use" &&
        object(tool) &&
        typeof tool.app === "string" &&
        tool.app
      ) {
        const offered = new Set(
          Array.isArray(meta.persist) ? meta.persist : [],
        );
        const subtitle = meta.warningSubtitle ?? meta.subtitle;
        this.native = freeze({
          app: tool.app,
          ...(typeof meta.riskLevel === "string"
            ? { riskLevel: meta.riskLevel }
            : {}),
          ...(typeof subtitle === "string" ? { subtitle } : {}),
          persist: (["session", "always"] as const).filter((value) =>
            offered.has(value),
          ),
        });
      }
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
            value._meta.persist as "session" | "always",
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
