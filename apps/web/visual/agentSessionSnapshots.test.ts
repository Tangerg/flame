import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-client/validation";
import {
  RUNTIME_AGENT_SESSION_SNAPSHOTS,
  RUNTIME_AGENT_SESSION_TAIL_EVENTS,
  VISUAL_AGENT_STATES,
  delegatedApprovalSnapshot,
} from "./agentSessionSnapshots";
describe("the visual agent fixtures", () => {
  it("parks the delegated tree before offering its root resume", () => {
    const snapshot = delegatedApprovalSnapshot();
    expect(snapshot.runs.find((run) => run.id === "run_root")?.status).toBe("waiting");
    expect(
      snapshot.runs.filter((run) => run.status !== "finished").map((run) => run.status),
    ).toEqual(["waiting", "waiting"]);
    expect(snapshot.runs.flatMap((run) => validateWire("RunRef", run))).toEqual([]);
    expect(snapshot.items.flatMap((item) => validateWire("Item", item))).toEqual([]);
  });

  it("keeps the narrative's final approval behind a waiting Runtime root", () => {
    const snapshot = RUNTIME_AGENT_SESSION_SNAPSHOTS.narrative;
    const group = snapshot.pendingInterruptSets[0]!;
    expect(group.interrupts).toHaveLength(1);
    expect(snapshot.runs.find((run) => run.id === group.rootRunId)).toMatchObject({
      status: "waiting",
    });
  });

  it("hold only items the Runtime could have sent", () => {
    const violations = VISUAL_AGENT_STATES.flatMap((state) =>
      RUNTIME_AGENT_SESSION_SNAPSHOTS[state].items.flatMap((item) =>
        validateWire("Item", item).map(
          (violation) => `${state}/${item.id}: ${violation.path} ${violation.detail}`,
        ),
      ),
    );

    expect(violations).toEqual([]);
  });

  it("hold only stream frames the Runtime could have sent", () => {
    const violations = VISUAL_AGENT_STATES.flatMap((state) =>
      RUNTIME_AGENT_SESSION_TAIL_EVENTS[state].flatMap((frame) =>
        validateWire("StreamEvent", frame.event).map(
          (violation) => `${state}/${frame.index}: ${violation.path} ${violation.detail}`,
        ),
      ),
    );

    expect(violations).toEqual([]);
  });

  it("hold only runs the Runtime could have sent", () => {
    const violations = VISUAL_AGENT_STATES.flatMap((state) =>
      RUNTIME_AGENT_SESSION_SNAPSHOTS[state].runs.flatMap((run) =>
        validateWire("RunRef", run).map(
          (violation) => `${state}/${run.id}: ${violation.path} ${violation.detail}`,
        ),
      ),
    );

    expect(violations).toEqual([]);
  });

  const DECLARED_RESULT_SHAPE: Record<
    string,
    "SearchResult" | "PatchResult" | "CommandResult" | "WebSearchResult"
  > = {
    glob: "SearchResult",
    grep: "SearchResult",
    apply_patch: "PatchResult",
    shell: "CommandResult",
    web_search: "WebSearchResult",
  };

  it("answer each declared tool with the result shape the Runtime sends", () => {
    const violations = VISUAL_AGENT_STATES.flatMap((state) =>
      RUNTIME_AGENT_SESSION_SNAPSHOTS[state].items.flatMap((item) => {
        if (item.type !== "toolCall" || item.status !== "completed") return [];
        const shape = DECLARED_RESULT_SHAPE[item.tool.name];
        if (shape === undefined) return [];
        const result = item.tool.result;
        if (result === undefined)
          return [`${state}/${item.id}: settled ${item.tool.name} with no result`];
        return validateWire(shape, typeof result === "string" ? JSON.parse(result) : result).map(
          (violation) => `${state}/${item.id}: ${violation.path} ${violation.detail}`,
        );
      }),
    );

    expect(violations).toEqual([]);
  });
});
