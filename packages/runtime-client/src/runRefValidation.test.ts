import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-client/validation";
import type { RunRef } from "@flame/runtime-client/wire";

function runningRoot(patch: Record<string, unknown> = {}): RunRef {
  return {
    id: "run_root",
    sessionId: "ses_1",
    status: "running",
    activeSegmentId: "seg_1",
    createdAt: "2026-08-12T08:00:00.000Z",
    metrics: { steps: 0, activeDurationMillis: 0 },
    provider: "openai",
    model: "gpt-5",
    ...patch,
  } as RunRef;
}

describe("generated RunRef validation", () => {
  it("accepts a complete live root Run", () => {
    expect(validateWire("RunRef", runningRoot())).toEqual([]);
  });

  it.each([
    ["status", runningRoot({ status: undefined })],
    ["creation time", runningRoot({ createdAt: undefined })],
    [
      "complete child lineage",
      runningRoot({ spawnedByItemId: "item_spawn", parentRunId: "run_root" }),
    ],
    ["root lineage absence", runningRoot({ rootRunId: "run_root" })],
    ["running segment", runningRoot({ activeSegmentId: undefined })],
    ["complete model identity", runningRoot({ model: undefined })],
    ["waiting segment absence", runningRoot({ status: "waiting" })],
    ["terminal facts", runningRoot({ status: "finished", activeSegmentId: undefined })],
    [
      "non-terminal facts absence",
      runningRoot({
        status: "waiting",
        activeSegmentId: undefined,
        finishedAt: "2026-08-12T08:00:02.000Z",
        outcome: { type: "completed" },
      }),
    ],
  ])("rejects a Run missing %s", (_case, run) => {
    expect(validateWire("RunRef", JSON.parse(JSON.stringify(run)))).not.toEqual([]);
  });
});
