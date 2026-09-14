import { describe, expect, it } from "vitest";
import { EMPTY_AGENT_SESSION_VIEW } from "./agentSessionView";
import { setTimelineEntry } from "./agentTimeline";

describe("setTimelineEntry", () => {
  it("updates a tool in place without reordering simultaneous observations", () => {
    const tool = { id: "tool", ts: 1, kind: "tool" as const, runId: "run" };
    const started = setTimelineEntry(tool)(EMPTY_AGENT_SESSION_VIEW);
    const sibling = setTimelineEntry({ ...tool, id: "sibling" })(started);
    const completed = setTimelineEntry({ ...tool, status: "ok" })(sibling);
    expect(completed.timeline).toEqual([
      { ...tool, status: "ok" },
      { ...tool, id: "sibling" },
    ]);
    expect(started.timeline).toEqual([tool]);
  });

  it("orders cold-read facts by server timestamp rather than fetch order", () => {
    const newestFirst = [
      { id: "new", ts: 3, kind: "run-start" as const, runId: "run_new" },
      { id: "old", ts: 1, kind: "run-start" as const, runId: "run_old" },
      { id: "middle", ts: 2, kind: "run-start" as const, runId: "run_middle" },
    ];

    const view = newestFirst.reduce(
      (current, entry) => setTimelineEntry(entry)(current),
      EMPTY_AGENT_SESSION_VIEW,
    );

    expect(view.timeline.map((entry) => entry.id)).toEqual(["old", "middle", "new"]);
  });

  it("retains the newest 500 facts even when older facts arrive last", () => {
    let view = EMPTY_AGENT_SESSION_VIEW;
    for (let ts = 600; ts >= 1; ts -= 1) {
      view = setTimelineEntry({
        id: `entry_${ts}`,
        ts,
        kind: "run-start",
        runId: `run_${ts}`,
      })(view);
    }

    expect(view.timeline).toHaveLength(500);
    expect(view.timeline[0]?.ts).toBe(101);
    expect(view.timeline.at(-1)?.ts).toBe(600);
  });
});
