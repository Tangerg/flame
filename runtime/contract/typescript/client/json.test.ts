import { describe, expect, it } from "vitest";
import { parseReviewedJSON } from "./json";

describe("reviewed plugin input", () => {
  it.each([
    '{"n":9007199254740993}',
    '{"n":1e20}',
    '{"n":1e999}',
    '{"n":1,"n":2}',
    '{"value":"\\ud800"}',
  ])("refuses lossy or ambiguous JSON %s", (text) => {
    expect(() => parseReviewedJSON(text)).toThrow(/JSON|Unicode|number|member/);
  });
  it("retains explicit string identifiers and ordinary JSON values", () => {
    expect(
      parseReviewedJSON('{"id":"9007199254740993","value":0.25,"empty":"","unset":null}'),
    ).toEqual({ id: "9007199254740993", value: 0.25, empty: "", unset: null });
  });
});
