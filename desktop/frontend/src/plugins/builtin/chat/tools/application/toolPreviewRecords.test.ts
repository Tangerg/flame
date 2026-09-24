import { describe, expect, it } from "vitest";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { recordedGrepRows, recordedReadLines } from "./toolPreviewRecords";

const read = (tool: Partial<ToolCall>): ToolCall => ({
  id: "t",
  runId: "r",
  name: "read",
  fn: "src/a.ts",
  fnKind: "path",
  args: "",
  status: "ok",
  ...tool,
});

describe("recordedReadLines", () => {
  it("numbers what was read from where the read started, not from line 1", () => {
    const recorded = recordedReadLines(
      read({ result: "alpha\nbeta\n", range: { start: 200, end: 201 } }),
      40,
    );
    expect(recorded?.lines).toEqual([
      { lineNumber: 200, text: "alpha" },
      { lineNumber: 201, text: "beta" },
    ]);
  });

  it("says nothing was recorded instead of reading the file as it is now", () => {
    expect(recordedReadLines(read({ result: undefined }), 40)).toBeNull();
  });

  it("counts what it leaves out", () => {
    const recorded = recordedReadLines(read({ result: "1\n2\n3\n4" }), 2);
    expect(recorded?.hidden).toBe(2);
  });
});

describe("recordedGrepRows", () => {
  it("does not substitute a fresh search for hits it cannot parse", () => {
    expect(recordedGrepRows(read({ name: "grep", result: "plain text" }), 4)).toBeNull();
  });
});
