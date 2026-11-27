import {
  IDEMPOTENCY_KEY_PATTERN,
  MAXIMUM_IDEMPOTENCY_KEY_CHARACTERS,
} from "@flame/runtime-client/wire";

const pattern = new RegExp(IDEMPOTENCY_KEY_PATTERN, "u");

export function isIdempotencyKey(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length <= MAXIMUM_IDEMPOTENCY_KEY_CHARACTERS &&
    pattern.test(value)
  );
}
