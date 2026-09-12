import { describe, expect, it } from "vitest";
import { TOOL_RESULT_SHAPES, toolResultShape } from "./toolResult";

const PAYLOAD: Record<(typeof TOOL_RESULT_SHAPES)[number], unknown> = {
  search: { hits: [{ path: "one.ts", line: 1, text: "match" }] },
  patch: { changes: [{ path: "one.ts", status: "modified" }] },
  webSearch: { results: [{ title: "Flame", url: "https://example.test" }] },
  command: { output: "ok", exitCode: 0 },
};

describe("toolResultShape", () => {
  it("names every declared shape, decoded or still a string", () => {
    for (const shape of TOOL_RESULT_SHAPES) {
      expect(toolResultShape(PAYLOAD[shape])).toBe(shape);
      expect(toolResultShape(JSON.stringify(PAYLOAD[shape]))).toBe(shape);
    }
  });

  it("answers nothing rather than guessing", () => {
    expect(toolResultShape(undefined)).toBeUndefined();
    expect(toolResultShape("")).toBeUndefined();
    expect(toolResultShape("not json at all")).toBeUndefined();
    expect(toolResultShape([1, 2, 3])).toBeUndefined();
    expect(toolResultShape({ exitCode: 0 })).toBeUndefined();
    expect(toolResultShape({ hits: "one" })).toBeUndefined();
  });
});
