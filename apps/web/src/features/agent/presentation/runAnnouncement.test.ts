import { type AgentRunView } from "@/features/agent/public/sessionView";
import { describe, expect, it } from "vitest";
import { runAnnouncement } from "@/features/agent/presentation/runAnnouncement";
const outcome = (patch: AgentRunView["outcome"]): AgentRunView["outcome"] => patch;

describe("runAnnouncement", () => {
  it("says what the turn is doing, never what it said", () => {
    expect(runAnnouncement("running", null)).toBe("running");
    expect(runAnnouncement("waiting", null)).toBe("needsInput");
    expect(runAnnouncement("idle", null)).toBeNull();
  });

  it("names how a finished turn ended, through the same mapping the notifier uses", () => {
    expect(runAnnouncement("finished", null)).toBe("finished");
    expect(runAnnouncement("finished", outcome({ type: "canceled" }))).toBe("canceled");
  });
});
