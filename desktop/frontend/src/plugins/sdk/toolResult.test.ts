import { describe, expect, it } from "vitest";
import { TOOL_RESULT_SHAPES, toolResultShape } from "./toolResult";

// The shape decides which preview renders a tool result, and every narrower accepts any
// object — they differ only in the property each one looks for. So the discriminator is the
// whole of the routing, and a payload that answers to two of them answers to the first.
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
    // A command whose output is absent is not a command result: the preview reads that field.
    expect(toolResultShape({ exitCode: 0 })).toBeUndefined();
    // Present but the wrong type, which is what a Runtime rename leaves behind.
    expect(toolResultShape({ hits: "one" })).toBeUndefined();
  });
});
