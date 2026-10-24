import { describe, expect, it } from "vitest";
import { ExactNumber, parseJSON } from "@flame/runtime-client";
import {
  canonicalJSON,
  digestJSON,
  Method,
  type MethodDescriptor,
} from "./contract";
import { compileSchema } from "./schema";
import wire from "./protocol.generated.json";

describe("published Go contract conformance", () => {
  for (const vector of wire.inputs)
    it(vector.source, () => {
      const value = parseJSON(vector.source);
      expect(canonicalJSON(value)).toBe(vector.canonical);
      expect(digestJSON(value)).toBe(vector.digest);
    });
  it("retains exact method identity and excludes presentation prose", () => {
    const descriptor = parseJSON(wire.receipt) as MethodDescriptor;
    const original = new Method(descriptor);
    expect(original.identity).toBe(wire.receiptIdentity);
    expect(
      new Method({ ...descriptor, description: "different prose" }).identity,
    ).toBe(original.identity);
    expect(new Method({ ...descriptor, name: "changed" }).identity).not.toBe(
      original.identity,
    );
    expect(new Method({ ...descriptor, kind: "command" }).identity).not.toBe(
      original.identity,
    );
    expect(new Method({ ...descriptor, target: "call" }).identity).not.toBe(
      original.identity,
    );
  });
  it("validates exact numbers through the schema engine without conversion", () => {
    const validate = compileSchema(
      parseJSON(
        '{"type":"integer","minimum":9007199254740993,"maximum":9223372036854775807}',
      ),
    );
    expect(() => validate(9007199254740993n)).not.toThrow();
    expect(() => validate(9007199254740992n)).toThrow();
    expect(() => validate(9223372036854775808n)).toThrow();
    expect(() => validate(new ExactNumber("9007199254740993.0"))).not.toThrow();
    expect(() => validate(new ExactNumber("9007199254740993.1"))).toThrow();
  });
  it("keeps enum and uniqueness numeric equality exact", () => {
    const validate = compileSchema(
      parseJSON('{"enum":[9007199254740993,1.0]}'),
    );
    expect(() => validate(9007199254740993n)).not.toThrow();
    expect(() => validate(9007199254740992n)).toThrow();
    expect(() => validate(1)).not.toThrow();
    const unique = compileSchema({ type: "array", uniqueItems: true });
    expect(() => unique([1, new ExactNumber("1.0")])).toThrow();
  });
  it("retains required/null/unknown-member and ref sibling constraints", () => {
    const validate = compileSchema({
      $defs: { id: { type: "string", minLength: 2 } },
      type: "object",
      required: ["id"],
      additionalProperties: false,
      properties: { id: { $ref: "#/$defs/id", maxLength: 3 } },
    });
    expect(() => validate({ id: "ok" })).not.toThrow();
    for (const value of [
      {},
      { id: null },
      { id: "long" },
      { id: "a" },
      { id: "ok", extra: false },
    ])
      expect(() => validate(value)).toThrow();
  });
  it("refuses unsupported or malformed schemas at construction", () => {
    for (const schema of [
      { type: "typo" },
      { minimum: "2" },
      { $ref: "https://example.com/schema" },
      { unevaluatedProperties: false },
    ])
      expect(() => compileSchema(schema)).toThrow();
  });
});
