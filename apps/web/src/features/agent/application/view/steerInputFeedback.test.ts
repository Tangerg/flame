import { describe, expect, it } from "vitest";
import { type Message } from "@/features/agent/public/sessionView";
import { steerInputFeedback } from "@/features/agent/application/view/steerInputFeedback";
const input: Message = { id: "item_original", role: "user", runId: null, blocks: [] };

describe("Steer input feedback from owner facts", () => {
  it.each(["pending", "unknown"] as const)(
    "does not call %s delivery accepted or rejected",
    (delivery) => {
      const message = { ...input, steer: { runId: "run_1", delivery } };
      expect(steerInputFeedback(message)).toBe(delivery);
      expect(steerInputFeedback({ ...message, runId: "run_1" })).toBe("applied");
    },
  );
  it("preserves accepted evidence despite a later inconclusive delivery observation", () => {
    const message = {
      ...input,
      steer: { runId: "run_1", receipt: "accepted" as const, delivery: "unknown" as const },
    };
    expect(steerInputFeedback(message)).toBe("accepted");
    expect(steerInputFeedback({ ...message, runId: "run_1" })).toBe("applied");
  });
  it("renders non-application only from a coherent terminal Session projection", () => {
    expect(
      steerInputFeedback({
        ...input,
        steer: { runId: "run_1", receipt: "accepted", application: "notApplied" },
      }),
    ).toBe("notApplied");
  });
  it("does not interpret retired material as input rejection", () => {
    expect(steerInputFeedback({ ...input, steer: { runId: "run_1", receipt: "retired" } })).toBe(
      "retired",
    );
    expect(steerInputFeedback(input)).toBeUndefined();
  });
});
