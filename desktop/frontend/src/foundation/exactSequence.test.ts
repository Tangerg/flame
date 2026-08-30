import { describe, expect, it } from "vitest";
import { ExactSequence } from "./exactSequence";

describe("ExactSequence", () => {
  it("keeps issuing distinct values beyond JavaScript's safe-integer boundary", () => {
    const boundary = BigInt(Number.MAX_SAFE_INTEGER);
    const sequence = new ExactSequence(boundary);

    expect(sequence.issue()).toBe(boundary + 1n);
    expect(sequence.issue()).toBe(boundary + 2n);
  });

  it("rejects an invalid restored state", () => {
    expect(() => new ExactSequence(-1n)).toThrow("Exact sequence cannot start below zero");
  });
});
