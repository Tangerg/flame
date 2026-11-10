import { ExactNumber } from "@frame/sdk/json";
import { describe, expect, it, vi } from "vitest";
import {
  array,
  integer,
  maxItems,
  projectWireValue,
  uniqueItems,
  type WireViolation,
} from "./wireCheck";

describe("array admission", () => {
  it("refuses excess capacity before inspecting elements", () => {
    const inspect = vi.fn();
    const violations: WireViolation[] = [];
    maxItems(2, array(inspect))(["first", "second", "excess"], "items", violations);
    expect(violations).toEqual([{ path: "items", detail: "expected at most 2 item(s)" }]);
    expect(inspect).not.toHaveBeenCalled();
  });

  it("retains element admission and numeric projection within capacity", () => {
    const check = maxItems(2, array(integer()));
    const violations: WireViolation[] = [];
    check([new ExactNumber("1.0"), "invalid"], "items", violations);
    expect(violations).toEqual([{ path: "items[1]", detail: "expected an integer" }]);
    expect(projectWireValue(check, [new ExactNumber("1.0")])).toEqual([1]);
  });

  it("retains the array type boundary", () => {
    const violations: WireViolation[] = [];
    maxItems(2, array(integer()))({}, "items", violations);
    expect(violations).toEqual([{ path: "items", detail: "expected an array" }]);
  });
});

describe("JSON item uniqueness", () => {
  it("admits a complete catalog of distinct tool identities", () => {
    const values = Array.from({ length: 2048 }, (_, index) => `tool_${index}`);
    const violations: WireViolation[] = [];
    uniqueItems()(values, "items", violations);
    expect(violations).toEqual([]);
  });

  it.each([
    { name: "numeric spelling", values: [1, new ExactNumber("1.0")] },
    {
      name: "large exact integers",
      values: [9007199254740993n, new ExactNumber("9007199254740993")],
    },
    {
      name: "object key order and numeric spelling",
      values: [
        { first: 1, second: "value" },
        { second: "value", first: new ExactNumber("1e0") },
      ],
    },
    {
      name: "captured date strings",
      values: [new Date("2026-11-10T00:00:00Z"), "2026-11-10T00:00:00.000Z"],
    },
    { name: "repeated strings", values: ["tool", "tool"] },
  ])("retains Frame's equality for $name", ({ values }) => {
    const violations: WireViolation[] = [];
    uniqueItems()(values, "items", violations);
    expect(violations).toEqual([{ path: "items", detail: "expected no repeated items" }]);
  });

  it("keeps unlike primitives and distinct exact numbers separate", () => {
    const violations: WireViolation[] = [];
    uniqueItems()(
      [
        null,
        "null",
        false,
        "false",
        0,
        "0",
        new ExactNumber("9007199254740993"),
        9007199254740992n,
      ],
      "items",
      violations,
    );
    expect(violations).toEqual([]);
  });
});
