import { describe, expect, it } from "vitest";
import { validateWire } from "@flame/runtime-client/validation";
import { projectAgentSessionSnapshot } from "@/features/agent/application/session/sessionSnapshot";
import { reduceAgentEvent } from "@/features/agent/application/fold/reducer";
import {
  AGENT_SESSION_TAIL_EVENTS,
  RUNTIME_AGENT_SESSION_SNAPSHOTS,
  RUNTIME_AGENT_SESSION_TAIL_EVENTS,
  VISUAL_AGENT_STATES,
  delegatedApprovalSnapshot,
  projectRuntimeAgentSnapshot,
} from "./agentSessionSnapshots";
describe("the visual agent fixtures", () => {
  it.each(["running", "tool-shells"] as const)(
    "presents the captured file body in %s instead of its transport envelope",
    (state) => {
      let view = projectAgentSessionSnapshot(
        projectRuntimeAgentSnapshot(RUNTIME_AGENT_SESSION_SNAPSHOTS[state]),
      );
      for (const event of AGENT_SESSION_TAIL_EVENTS[state]) view = reduceAgentEvent(view, event);
      const read = Object.values(view.toolCalls).find((call) => call.name === "read");

      expect(read?.result).toBe(
        [
          "package session",
          "",
          "func (s *Store) Commit(ctx context.Context, session *Session, records []Record, opts CommitOptions) (Receipt, error) {",
          "\tif err := s.flushLocked(); err != nil {",
          '\t\treturn Receipt{}, fmt.Errorf("commit: %w", err)',
          "\t}",
        ].join("\n"),
      );
      expect(read?.lines).toBe(6);
    },
  );

  it("presents captured command output and exit status in the shell reader", () => {
    const view = projectAgentSessionSnapshot(
      projectRuntimeAgentSnapshot(RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-shells"]),
    );
    const command = view.toolCalls.item_shells_command;

    expect(command?.command).toBe("go test ./internal/session/...");
    expect(command?.exitCode).toBe(1);
    expect(command?.result?.startsWith("\u001b[1m=== RUN   TestCommitAtomicity")).toBe(true);
  });

  it("presents the Runtime's recorded change receipt independently of abbreviated patch input", () => {
    const view = projectAgentSessionSnapshot(
      projectRuntimeAgentSnapshot(RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-shells"]),
    );
    const edit = view.toolCalls.item_shells_edit;

    expect(edit?.files).toBe(3);
    expect(edit?.changes?.map((change) => change.status)).toEqual(["modified", "moved", "deleted"]);
    expect(edit?.changes?.[1]?.from).toBe("runtime/internal/session/store_atomicity.go");
    expect([edit?.added, edit?.removed]).toEqual([0, 0]);
  });

  it("retains each Skill page's incoming scope independently of its outgoing continuation", () => {
    const snapshot = RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-skill-pages"];
    const view = projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot));
    const calls = snapshot.items
      .filter((item) => item.type === "toolCall")
      .filter((item) => item.tool.name === "load_skill");
    expect(calls).toHaveLength(3);
    expect(calls.map((call) => JSON.parse(view.toolCalls[call.id]!.args))).toEqual(
      calls.map((call) => call.tool.arguments),
    );
    expect(calls.map((call) => call.tool.arguments.cursor)).toEqual([
      undefined,
      { offset: 134, snapshot: "4b8c42fdc788c551938ea8f286970c3cad4d181330cbf0940d5d19550de01bac" },
      { offset: 266, snapshot: "4b8c42fdc788c551938ea8f286970c3cad4d181330cbf0940d5d19550de01bac" },
    ]);
    const resource = snapshot.items.find((item) => item.id === "item_skill_resource_continued");
    if (resource?.type !== "toolCall") throw new Error("Missing recorded resource continuation");
    expect(JSON.parse(view.toolCalls[resource.id]!.args)).toEqual(resource.tool.arguments);
    expect(JSON.parse(view.toolCalls[resource.id]!.result!)).toEqual({
      content: "\n- Confirm the captured output before proceeding.\n",
    });
  });

  it("retains the current Skill list and instruction pages through the real fold", () => {
    const snapshot = RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-agentic"];
    const view = projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot));
    const list = JSON.parse(view.toolCalls.item_agentic_list_skills!.result!);
    const instructions = JSON.parse(view.toolCalls.item_agentic_load_skill!.result!);

    expect(list).toEqual({
      skills: [
        {
          name: "review-diff",
          description: "Read a change the way a reviewer does, worst risk first.",
        },
        {
          name: "trace-flaky-test",
          description: "Find the shared state behind a test that passes alone.",
        },
      ],
      next_cursor: {
        offset: 2,
        snapshot: "861f411f0c0bf55ee4fe9596dc8e23ea96dd61243d2cb09e26191ed32cf3fee3",
      },
    });
    expect(instructions).toEqual({
      content:
        "Start from the riskiest hunk, not the first one.\nName the invariant the change could break, then look for the test that would catch it.\nA diff with no test change is a claim that nothing observable changed. Check it.",
      next_cursor: {
        offset: 216,
        snapshot: "a83a887aee84d11218a8dbfc69e4f1a35325ba7ccffaa894284945f0363bfd5e",
      },
    });
  });

  it("projects stored-output identity and the complete original read window", () => {
    const snapshot = RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-tail"];
    const recorded = snapshot.items.find((item) => item.id === "item_last_read_tool_result");
    if (recorded?.type !== "toolCall") throw new Error("Missing recorded stored-output read");
    const view = projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot));
    const call = view.toolCalls[recorded.id]!;

    expect(recorded.tool.arguments).toEqual({ result_id: "BLOB234" });
    expect(call.fn).toBe("BLOB234");
    expect(call.fnKind).toBeUndefined();
    expect(call.result).toBe(recorded.tool.result);
    expect(call.result).toContain("66 bytes total, showing bytes 0–66");
  });

  it("retains a recorded Skill-resource page through the real fold", () => {
    const snapshot = RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-tail"];
    const view = projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot));
    const call = view.toolCalls.item_last_skill_resource!;

    expect(call.fn).toBe("review-diff/checklist.md");
    expect(JSON.parse(call.result!)).toEqual({
      content:
        "- Riskiest hunk first.\n- Name the invariant.\n- Find the test that would catch it.",
      next_cursor: {
        offset: 81,
        snapshot: "6b31fb380eb6623db513d35a97e10b89d97185e1b53aef3449e684c63b1c960c",
      },
    });
  });

  it("retains unknown tool arguments for secondary inspection through the real fold", () => {
    const snapshot = RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-agentic"];
    const recorded = snapshot.items.find((item) => item.id === "item_agentic_inspect_workspace");
    if (recorded?.type !== "toolCall") throw new Error("Missing recorded workspace inspection");
    const view = projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot));
    const call = view.toolCalls[recorded.id]!;

    expect(call.args).not.toBe("");
    expect(JSON.parse(call.args)).toEqual(recorded.tool.arguments);
    expect(JSON.parse(call.result!)).toEqual(JSON.parse(recorded.tool.result as string));
  });

  it.each([
    ["tool-search", "item_search_glob", "runtime/**/*_test.go"],
    ["tool-remote", "item_remote_web_search", "sqlite wal checkpoint starvation"],
  ] as const)("retains the searched object in %s summaries", (state, id, query) => {
    const view = projectAgentSessionSnapshot(
      projectRuntimeAgentSnapshot(RUNTIME_AGENT_SESSION_SNAPSHOTS[state]),
    );
    expect(view.toolCalls[id]?.fn).toBe(query);
  });

  it("retains each recorded delegation's purpose in its projected label", () => {
    const snapshot = RUNTIME_AGENT_SESSION_SNAPSHOTS.delegated;
    const view = projectAgentSessionSnapshot(projectRuntimeAgentSnapshot(snapshot));
    const calls = snapshot.items
      .filter((item) => item.type === "toolCall")
      .filter((item) => item.tool.name === "delegate_task");
    expect(calls).not.toHaveLength(0);
    expect(calls.map((item) => view.toolCalls[item.id]?.fn)).toEqual(
      calls.map((item) => item.tool.arguments.summary),
    );
  });

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

  it("project the narrative's reads with their file paths and original bodies", () => {
    const view = projectAgentSessionSnapshot(
      projectRuntimeAgentSnapshot(RUNTIME_AGENT_SESSION_SNAPSHOTS.narrative),
    );
    const reads = ["item_n_read", "item_n_read_2", "item_n_read_3"].map((id) => {
      const tool = view.toolCalls[id];
      return { label: tool?.fn, kind: tool?.fnKind, output: tool?.result };
    });

    expect(reads).toEqual([
      {
        label: "src/checkout/checkout.tsx",
        kind: "path",
        output: "export function Checkout() {\n  …\n}\n",
      },
      {
        label: "/Users/visual/scope/apps/web/src/features/chat/tools/ui/ToolGroup.tsx",
        kind: "path",
        output: "export function ToolGroup() {\n  …\n}\n",
      },
      {
        label: "src/checkout/api/pay.ts",
        kind: "path",
        output: "export async function pay() {\n  …\n}\n",
      },
    ]);
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

  const DECLARED_RESULT_SHAPE: Record<string, "SearchResult" | "PatchResult" | "CommandResult"> = {
    glob: "SearchResult",
    grep: "SearchResult",
    apply_patch: "PatchResult",
    shell: "CommandResult",
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
