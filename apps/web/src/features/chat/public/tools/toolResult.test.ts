import { expect, it } from "vitest";
import { patchToolResult, commandToolResult } from "@/features/chat/public/tools/toolResult";

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
