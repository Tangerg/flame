import { expect, it } from "vitest";
import { parseJSON } from "@frame/sdk/json";
import {
  Capability,
  canonicalJSON,
  declareMethod,
  digestJSON,
} from "./contract.ts";
import wire from "./protocol.generated.json";

it("agrees with the Go publisher on portable methods and Flame capability identities", () => {
  const capability = new Capability(parseJSON(wire.capability));
  expect(capability.identity).toBe(wire.capabilityIdentity);
  expect(canonicalJSON(capability.descriptor)).toBe(wire.capability);
  const receipt = declareMethod(parseJSON(wire.receipt));
  expect(receipt.identity).toBe(wire.receiptIdentity);
  for (const input of wire.inputs) {
    const value = parseJSON(input.source);
    expect(canonicalJSON(value)).toBe(input.canonical);
    expect(digestJSON(value)).toBe(input.digest);
  }
});

it("keeps Flame composition and scope separate from Frame method identity", () => {
  const descriptor = new Capability(parseJSON(wire.capability)).descriptor;
  const original = new Capability(descriptor);
  const selection = new Capability({ ...descriptor, composition: "selection" });
  const scoped = new Capability({ ...descriptor, scope: "workspace" });
  expect(selection.identity).not.toBe(original.identity);
  expect(scoped.identity).not.toBe(original.identity);
  const method = descriptor.methods[0]!;
  for (const capability of [selection, scoped]) {
    expect(capability.portable.method(method.name).identity).toBe(
      original.portable.method(method.name).identity,
    );
  }
  const documented = new Capability({
    ...descriptor,
    methods: descriptor.methods.map((method) => ({
      ...method,
      description: "Revised prose.",
    })),
  });
  expect(documented.identity).toBe(original.identity);
});

it("delegates exact schema admission to the released Frame method", () => {
  const receipt = declareMethod(parseJSON(wire.receipt));
  expect(() =>
    receipt.admitInput({
      id: "command",
      method: "read",
      inputDigest: "a".repeat(64),
    }),
  ).not.toThrow();
  expect(() =>
    receipt.admitInput({
      id: "command",
      method: "read",
      inputDigest: "invalid",
    }),
  ).toThrow();
  expect(() =>
    receipt.admitInput({
      id: "command",
      method: "read",
      inputDigest: "a".repeat(64),
      unexpected: true,
    }),
  ).toThrow();
});
