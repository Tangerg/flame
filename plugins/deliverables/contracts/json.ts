import { isJSONNumber, projectNumber } from "@qimu/sdk/json";

// These authoring DTOs promise finite JavaScript numbers. Qimu owns whether
// an exact source number is representable; this walk only projects containers.
export function projectNumbers(value: unknown): unknown {
  if (isJSONNumber(value)) return projectNumber(value);
  if (Array.isArray(value)) return value.map(projectNumbers);
  if (value !== null && typeof value === "object")
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, projectNumbers(item)]),
    );
  return value;
}
