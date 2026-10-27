import { parseJSON, projectNumber } from "@flame/runtime-client/json";
import { Temporal } from "@js-temporal/polyfill";
import { z } from "zod/v4";
import { finiteNumber, InvalidPresentation, limits } from "./expression";

export const fieldRulesSchema = z
  .strictObject({
    required: z.boolean().optional(),
    minLength: z.number().int().min(0).max(limits.text).optional(),
    maxLength: z.number().int().min(0).max(limits.text).optional(),
    minimum: finiteNumber.optional(),
    maximum: finiteNumber.optional(),
  })
  .superRefine((rules, context) => {
    if (
      rules.minLength !== undefined &&
      rules.maxLength !== undefined &&
      rules.minLength > rules.maxLength
    ) {
      context.addIssue({
        code: "custom",
        message: "minimum length exceeds maximum length",
      });
    }
    if (
      rules.minimum !== undefined &&
      rules.maximum !== undefined &&
      rules.minimum > rules.maximum
    ) {
      context.addIssue({ code: "custom", message: "minimum exceeds maximum" });
    }
  });

export type FieldRules = z.infer<typeof fieldRulesSchema>;
export type FieldKind = "text" | "number" | "email" | "date";
export type ValidationIssue = {
  code:
    | "required"
    | "number"
    | "email"
    | "date"
    | "dateMinimum"
    | "dateMaximum"
    | "minLength"
    | "maxLength"
    | "minimum"
    | "maximum";
  limit?: number | string;
};

export function validateField(
  value: unknown,
  rules: FieldRules = {},
  kind: FieldKind = "text",
  dateBounds: { min?: string; max?: string } = {},
): ValidationIssue | undefined {
  const empty =
    value === null ||
    value === undefined ||
    value === "" ||
    (Array.isArray(value) && value.length === 0);
  if (empty) return rules.required ? { code: "required" } : undefined;
  if (rules.required && typeof value === "string" && !value.trim())
    return { code: "required" };
  if (typeof value === "string" || Array.isArray(value)) {
    if (rules.minLength !== undefined && value.length < rules.minLength)
      return { code: "minLength", limit: rules.minLength };
    if (rules.maxLength !== undefined && value.length > rules.maxLength)
      return { code: "maxLength", limit: rules.maxLength };
  }
  if (
    kind === "email" &&
    (typeof value !== "string" || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/u.test(value))
  )
    return { code: "email" };
  if (kind === "date") {
    if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/u.test(value))
      return { code: "date" };
    try {
      const parsed = Temporal.PlainDate.from(value);
      if (
        dateBounds.min !== undefined &&
        Temporal.PlainDate.compare(parsed, dateBounds.min) < 0
      )
        return { code: "dateMinimum", limit: dateBounds.min };
      if (
        dateBounds.max !== undefined &&
        Temporal.PlainDate.compare(parsed, dateBounds.max) > 0
      )
        return { code: "dateMaximum", limit: dateBounds.max };
    } catch (error) {
      if (error instanceof RangeError) return { code: "date" };
      throw error;
    }
  }
  if (kind === "number") {
    let number: number;
    try {
      number = projectNumber(
        typeof value === "string" ? parseJSON(value) : value,
      );
      finiteNumber.parse(number);
    } catch (error) {
      if (
        error instanceof SyntaxError ||
        error instanceof TypeError ||
        error instanceof RangeError ||
        error instanceof z.ZodError
      )
        return { code: "number" };
      throw error;
    }
    if (rules.minimum !== undefined && number < rules.minimum)
      return { code: "minimum", limit: rules.minimum };
    if (rules.maximum !== undefined && number > rules.maximum)
      return { code: "maximum", limit: rules.maximum };
  }
  return undefined;
}

export type URLKind = "link" | "image" | "media";

export function safeURL(value: string, kind: URLKind = "link"): string {
  if (
    value.length > 4096 ||
    value !== value.trim() ||
    Array.from(value).some((character) => {
      const code = character.codePointAt(0)!;
      return code <= 0x20 || code === 0x7f;
    }) ||
    /%(?:[01][0-9a-f]|7f)/iu.test(value)
  ) {
    throw new InvalidPresentation("unsafe URL");
  }
  let url: URL;
  try {
    url = new URL(value);
  } catch (error) {
    if (error instanceof TypeError)
      throw new InvalidPresentation("invalid URL", error);
    throw error;
  }
  const allowed = kind === "link" ? ["https:", "http:", "mailto:"] : ["https:"];
  if (
    !allowed.includes(url.protocol) ||
    url.username ||
    url.password ||
    (url.protocol !== "mailto:" && !url.hostname)
  ) {
    throw new InvalidPresentation("unsafe URL protocol or credentials");
  }
  return value;
}

export function isSafeURL(value: string, kind: URLKind = "link"): boolean {
  try {
    safeURL(value, kind);
    return true;
  } catch (error) {
    if (error instanceof InvalidPresentation) return false;
    throw error;
  }
}
