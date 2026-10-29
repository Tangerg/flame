import { METHODS } from "./methods";
import { LIMITS } from "./limits";
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
  for (const { value, accepted } of [
    { value: { leaseSeconds: LIMITS.minLeaseSeconds }, accepted: true },
    { value: { leaseSeconds: LIMITS.maxLeaseSeconds }, accepted: true },
    { value: { leaseSeconds: LIMITS.defaultLeaseSeconds }, accepted: true },
    { value: { leaseSeconds: LIMITS.minLeaseSeconds - 1 }, accepted: false },
    { value: { leaseSeconds: LIMITS.maxLeaseSeconds + 1 }, accepted: false },
    { value: { leaseSeconds: LIMITS.minLeaseSeconds + 0.5 }, accepted: false },
    {
      value: { leaseSeconds: LIMITS.minLeaseSeconds, approved: true },
      accepted: false,
    },
  ]) {
    const valid = schemas.open.safeParse(value).success;
    expect(valid).toBe(accepted);
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
