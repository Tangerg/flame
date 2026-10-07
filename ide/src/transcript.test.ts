import { describe, expect, it } from "vitest";
import type { Item } from "@flame/runtime-contract/wire";
import { itemText } from "./transcript";

const failedCall: Item = {
  type: "toolCall",
  id: "item_1",
  runId: "run_1",
  startedAt: "2026-10-07T00:00:00Z",
  status: "incomplete",
  approvalDecision: "deny",
  error: { type: "denied_by_user", detail: "the person declined the edit" },
  tool: { name: "write_file", arguments: { path: "a.txt" } },
};

describe("IDE transcript", () => {
  it("shows why a tool call did not complete instead of only its arguments", () => {
    const text = itemText(failedCall);
    expect(text).toContain("Approval denied");
    expect(text).toContain("Error denied_by_user: the person declined the edit");
  });
});
