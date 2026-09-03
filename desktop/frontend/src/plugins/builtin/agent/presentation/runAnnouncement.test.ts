import type { AgentRunView } from "@/plugins/sdk/types/agentSessionView";
import { describe, expect, it } from "vitest";
import { runAnnouncement, runAnnouncementKey } from "./runAnnouncement";

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
    expect(runAnnouncement("finished", outcome({ type: "maxSteps" }))).toBe("limit");
  });

  it("has a catalog key for every state it can be in", () => {
    for (const status of ["running", "waiting", "finished", "idle"] as const) {
      const key = runAnnouncementKey(runAnnouncement(status, null));
      expect(status === "idle" ? key === null : key?.startsWith("announce.")).toBe(true);
    }
  });
});
