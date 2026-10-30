import { describe, expect, it } from "vitest";
import {
  toolResultShape,
  patchToolResult,
  commandToolResult,
} from "@/features/chat/public/tools/toolResult";
import { RESULT_SHAPES, type ResultShape } from "@/foundation/protocolValues.generated";
const PAYLOAD: Record<ResultShape, unknown> = {
  search: { hits: [{ path: "one.ts", line: 1, text: "match" }] },
  patch: { changes: [{ path: "one.ts", status: "modified" }] },
  webSearch: { results: [{ title: "Flame", url: "https://example.test" }] },
  command: { output: "ok", exitCode: 0 },
};

describe("toolResultShape", () => {
  it("names every declared shape, decoded or still a string", () => {
    for (const shape of RESULT_SHAPES) {
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

it("refuses malformed change summaries without fabricating counts", () => {
  for (const changes of ["oops", [null], [{ path: 7 }], [{ path: "x", status: "modified" }]]) {
    expect(patchToolResult({ changes })).toBeUndefined();
  }
  expect(
    patchToolResult('{"changes":[{"path":"one.ts","status":"modified","added":1.0,"removed":0}]}'),
  ).toEqual({ changes: [{ path: "one.ts", status: "modified", added: 1, removed: 0 }] });
  expect(
    patchToolResult(
      '{"changes":[{"path":"one.ts","status":"modified","added":9007199254740993,"removed":0}]}',
    ),
  ).toBeUndefined();
});

it("projects numeric result fields only when their values remain exact", () => {
  expect(commandToolResult('{"output":"ok","exitCode":1.0}')).toEqual({
    output: "ok",
    exitCode: 1,
  });
  expect(commandToolResult('{"output":"ok","exitCode":9007199254740993}')).toEqual({
    output: "ok",
  });
});

it("retains an exit receipt when command output arrived through the stream", () => {
  expect(commandToolResult({ exitCode: 0 })).toEqual({ exitCode: 0 });
  expect(commandToolResult({ output: 7, exitCode: "0" })).toBeUndefined();
});

it("keeps recognition priority in the client adapter", () => {
  const value: Record<string, unknown> = { hits: [], changes: [], results: [], output: "ok" };
  expect(toolResultShape(value)).toBe("search");
  delete value.hits;
  expect(toolResultShape(value)).toBe("patch");
  delete value.changes;
  expect(toolResultShape(value)).toBe("webSearch");
  delete value.results;
  expect(toolResultShape(value)).toBe("command");
});
