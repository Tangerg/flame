import { METHODS } from "./methods";
import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import { Capability, parseJSON, type CapabilityDescriptor } from "@qimu/node";
import descriptor, { schemas } from "./capability";

it("publishes projections of the one descriptor owner", () => {
  const generated = parseJSON(
    readFileSync(
      new URL(
        "../package/io.github.tangerg.flame/contracts/capability.generated.json",
        import.meta.url,
      ),
      "utf8",
    ),
  ) as CapabilityDescriptor;
  expect(new Capability(generated).identity).toBe(
    new Capability(descriptor).identity,
  );
});

it("shares structural admission for backend calls and published contracts", () => {
  const contract = new Capability(descriptor);
  for (const value of [
    { leaseSeconds: 60 },
    { leaseSeconds: 3600 },
    { leaseSeconds: 0 },
    { leaseSeconds: 3601 },
    { leaseSeconds: 60, approved: true },
  ]) {
    const valid = schemas.open.safeParse(value).success;
    if (valid)
      expect(() =>
        contract.method(METHODS.open).validateInput(value),
      ).not.toThrow();
    else
      expect(() =>
        contract.method(METHODS.open).validateInput(value),
      ).toThrow();
  }
});
