import { canonicalJSON } from "@flame/backend-sdk/mcp";
import { isJSONNumber } from "@frame/sdk/json";
import { z } from "zod";
import { LIMITS, MODEL_LIMITS } from "../contracts/limits";
import { refusal } from "./refusal";

export const selection = {
  path: z
    .array(z.string().max(256))
    .max(32)
    .optional()
    .describe(
      "Exact property names or decimal array indexes in the documented inspection view. Use [] to inspect its root. Omit for a summary.",
    ),
  offset: z
    .number()
    .int()
    .min(0)
    .max(LIMITS.inputBytes)
    .optional()
    .describe(
      "Continue the selected collection or string at its returned nextOffset.",
    ),
  limit: z
    .number()
    .int()
    .min(1)
    .max(MODEL_LIMITS.maximumTextCharacters)
    .optional()
    .describe(
      "At most 32 collection entries or 4096 Unicode code points. The byte budget can return fewer entries with nextOffset.",
    ),
};

type Selection = { path?: string[]; offset?: number; limit?: number };

export function size(value: unknown): number {
  return Buffer.byteLength(canonicalJSON(value));
}

function record(value: unknown): value is Record<string, unknown> {
  return (
    value !== null &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    !isJSONNumber(value)
  );
}

function kind(value: unknown): string {
  return value === null
    ? "null"
    : isJSONNumber(value)
      ? "number"
      : Array.isArray(value)
        ? "array"
        : typeof value;
}

function omitted(value: unknown) {
  return {
    omitted: true,
    type: kind(value),
    bytes: size(value),
    ...(Array.isArray(value)
      ? { length: value.length }
      : record(value)
        ? { keys: Object.keys(value).length }
        : typeof value === "string"
          ? { characters: Array.from(value).length }
          : {}),
  };
}

export function selectedPath(selected: Selection): string[] {
  const path = selected.path ?? [];
  if (size(path) > MODEL_LIMITS.pathBytes)
    throw refusal(
      "projection_path_capacity",
      "inspection path exceeds its byte capacity",
    );
  if (
    selected.path === undefined &&
    (selected.offset !== undefined || selected.limit !== undefined)
  )
    throw refusal(
      "invalid_projection",
      "offset and limit require an explicit path",
    );
  return path;
}

function atPath(value: unknown, path: string[]): unknown {
  let source = value;
  for (const key of path) {
    if (Array.isArray(source)) {
      if (
        !/^(?:0|[1-9][0-9]*)$/u.test(key) ||
        !Number.isSafeInteger(Number(key)) ||
        Number(key) >= source.length
      )
        throw refusal(
          "inspection_path_not_found",
          "array index is outside the exact retained inspection",
        );
      source = source[Number(key)];
    } else if (record(source) && Object.hasOwn(source, key))
      source = source[key];
    else
      throw refusal(
        "inspection_path_not_found",
        "path is not present in the exact retained inspection",
      );
  }
  return source;
}

function textPage(
  source: string,
  path: string[],
  selected: Selection,
): Record<string, unknown> {
  const offset = selected.offset ?? 0;
  const characters = Array.from(source);
  if (offset > characters.length)
    throw refusal(
      "invalid_projection",
      "text offset exceeds the retained string",
    );
  let end = Math.min(
    characters.length,
    offset + (selected.limit ?? MODEL_LIMITS.textCharacters),
  );
  do {
    const result = {
      path,
      type: "string",
      unit: "unicode-code-points",
      offset,
      total: characters.length,
      text: characters.slice(offset, end).join(""),
      complete: offset === 0 && end === characters.length,
      ...(end < characters.length ? { nextOffset: end } : {}),
    };
    if (size(result) <= MODEL_LIMITS.projectionBytes) return result;
    end -= 1;
  } while (end > offset);
  throw refusal(
    "projection_capacity",
    "one text position exceeds the model projection capacity",
  );
}

function collectionPage(
  source: unknown[] | Record<string, unknown>,
  path: string[],
  selected: Selection,
): Record<string, unknown> {
  if (
    record(source) &&
    selected.offset === undefined &&
    selected.limit === undefined
  ) {
    const result = { path, type: "object", value: source, complete: true };
    if (size(result) <= MODEL_LIMITS.projectionBytes) return result;
  }
  const keys = Array.isArray(source)
    ? source.map((_, index) => String(index))
    : Object.keys(source).sort();
  const offset = selected.offset ?? 0;
  if (offset > keys.length)
    throw refusal(
      "invalid_projection",
      "collection offset exceeds the retained value",
    );
  const limit = selected.limit ?? MODEL_LIMITS.entries;
  if (limit > MODEL_LIMITS.maximumEntries)
    throw refusal(
      "invalid_projection",
      "collection pages admit at most 32 entries",
    );
  const entries: Array<Record<string, unknown>> = [];
  let next = offset;
  const result = () => ({
    path,
    type: kind(source),
    offset,
    total: keys.length,
    entries,
    complete:
      offset === 0 &&
      next === keys.length &&
      entries.every((entry) => !entry.omitted),
    ...(next < keys.length ? { nextOffset: next } : {}),
  });
  for (const key of keys.slice(offset, offset + limit)) {
    const value = Array.isArray(source) ? source[Number(key)] : source[key];
    const entry = {
      key,
      ...(size(value) <= MODEL_LIMITS.entryBytes ? { value } : omitted(value)),
    };
    entries.push(entry);
    next += 1;
    if (size(result()) > MODEL_LIMITS.projectionBytes) {
      entries.pop();
      next -= 1;
      if (!entries.length)
        throw refusal(
          "projection_capacity",
          "one retained key exceeds the model projection capacity",
        );
      break;
    }
  }
  return result();
}

export function project(
  value: unknown,
  selected: Selection = {},
): Record<string, unknown> {
  const path = selectedPath(selected);
  const source = atPath(value, path);
  if (typeof source === "string") return textPage(source, path, selected);
  if (Array.isArray(source) || record(source))
    return collectionPage(source, path, selected);
  if ((selected.offset ?? 0) !== 0 || selected.limit !== undefined)
    throw refusal("invalid_projection", "a scalar has no page offsets");
  return { path, type: kind(source), value: source, complete: true };
}
