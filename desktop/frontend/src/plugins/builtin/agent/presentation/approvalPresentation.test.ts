import { describe, expect, it } from "vitest";
import { canSubmitApproval } from "../public/messagePresentation";

describe("approvalPresentation", () => {
  it("allows submit only while the approval awaits an answer", () => {
    expect(canSubmitApproval({ resumeRunId: "run", itemId: "item", pending: null })).toBe(true);
    expect(canSubmitApproval({ resumeRunId: "run", itemId: "item", pending: "approved" })).toBe(
      false,
    );
    expect(canSubmitApproval({ resumeRunId: undefined, itemId: "item", pending: null })).toBe(
      false,
    );
  });
});
