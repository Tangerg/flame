import { describe, expect, it } from "vitest";
import { ExactNumber, parseJSON } from "@qimu/sdk/json";
import {
  Capability,
  canonicalJSON,
  digestJSON,
  Method,
  validCommand,
  validToken,
  type MethodDescriptor,
  type CapabilityDescriptor,
} from "./contract.ts";
import { compileSchema } from "./schema.ts";
import wire from "./protocol.generated.json";

it("uses the Go-owned byte budgets for protocol tokens and command identities", () => {
  expect(validCommand("a".repeat(wire.constants.maxCommandIDBytes))).toBe(true);
  expect(validCommand("a".repeat(wire.constants.maxCommandIDBytes + 1))).toBe(
    false,
  );
  expect(validToken("a".repeat(wire.limits.maxTokenBytes))).toBe(true);
  expect(validToken("a".repeat(wire.limits.maxTokenBytes + 1))).toBe(false);
  expect(validCommand("two words")).toBe(true);
  expect(validToken("two words")).toBe(false);
  expect(
    validToken("界".repeat(Math.floor(wire.limits.maxTokenBytes / 3) + 1)),
  ).toBe(false);
});

describe("published Go contract conformance", () => {
  for (const vector of wire.inputs)
    it(vector.source, () => {
      const value = parseJSON(vector.source);
      expect(canonicalJSON(value)).toBe(vector.canonical);
      expect(digestJSON(value)).toBe(vector.digest);
    });
  it("retains Go token admission and Unicode method ordering", () => {
    const descriptor = parseJSON(wire.capability) as CapabilityDescriptor;
    const contract = new Capability({
      ...descriptor,
      methods: [...descriptor.methods].reverse(),
    });
    expect(contract.identity).toBe(wire.capabilityIdentity);
    expect(contract.descriptor.methods.map((method) => method.name)).toEqual(
      descriptor.methods.map((method) => method.name),
    );
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
  it("allows absent optional properties while enforcing present and required values", () => {
    const validate = compileSchema({
      type: "object",
      required: ["id"],
      additionalProperties: false,
      properties: {
        id: { type: "string" },
        reason: { type: "string", enum: ["user", "expired"] },
        marker: { const: "accepted" },
        timeout_ms: { type: "integer", minimum: 1 },
        operation: {
          type: "object",
          required: ["id"],
          additionalProperties: false,
          properties: { id: { type: "string" }, tag: { const: "running" } },
        },
      },
    });
    for (const value of [
      { id: "session" },
      { id: "session", operation: { id: "operation" } },
      { id: "session", reason: "user", marker: "accepted", timeout_ms: 1 },
    ])
      expect(() => validate(value)).not.toThrow();
    for (const value of [
      undefined,
      {},
      { id: "session", reason: null },
      { id: "session", reason: "invalid" },
      { id: "session", marker: "invalid" },
      { id: "session", timeout_ms: 0 },
      { id: "session", operation: {} },
      { id: "session", operation: { id: "operation", tag: "invalid" } },
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
