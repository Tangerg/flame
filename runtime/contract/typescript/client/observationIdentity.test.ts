import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-contract/validate";

describe.each(["ModelInvocation", "ToolAttempt"] as const)("%s call identity", (type) => {
  const observation = {
    runId: "run_1",
    segmentId: "seg_1",
    state: "started",
    startedAt: "2026-09-29T08:27:31Z",
    ...(type === "ToolAttempt" ? { itemId: "item_1" } : {}),
  };

  it.each(["model:root:19", "tool:root:1", "AZaz09._:-", "x".repeat(256)])(
    "preserves the executor identity %s",
    (callId) => {
      expect(validateWire(type, { ...observation, callId })).toEqual([]);
    },
  );

  it.each([
    "",
    "call~1",
    "call/1",
    "call%3A1",
    "call 1",
    "call\n",
    "call\0",
    "调用",
    "x".repeat(257),
  ])("rejects identities outside the executor envelope: %j", (callId) => {
    expect(validateWire(type, { ...observation, callId }).length).toBeGreaterThan(0);
  });
});
