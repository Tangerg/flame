import { describe, expect, it } from "vitest";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { planRenderUnits, waveStepCount, type MessageRenderUnit } from "./messageRenderUnits";

const NO_AWAITING: ReadonlyMap<string, string> = new Map();

const text = (value: string, status: "running" | "complete" = "complete"): ContentBlock => ({
  kind: "text",
  text: value,
  status,
});

const reasoning = (status: "running" | "complete" = "complete"): ContentBlock => ({
  kind: "reasoning",
  reasoningId: "r1",
  text: "why",
  status,
});

const toolBlock = (toolCallId: string): ContentBlock => ({ kind: "tool", toolCallId });

const tool = (
  id: string,
  name: string,
  safetyClass: ToolCall["safetyClass"] = "safe",
  status: ToolCall["status"] = "ok",
): ToolCall => ({
  id,
  runId: "run_1",
  name,
  fn: name,
  args: "",
  status,
  safetyClass,
});

const TOOLS: Record<string, ToolCall> = {
  read: tool("read", "read"),
  grep: tool("grep", "grep"),
  shell: tool("shell", "shell", "exec"),
  edit: tool("edit", "edit", "write"),
};

const shape = (units: MessageRenderUnit[]): unknown =>
  units.map((unit) => {
    if (unit.kind === "wave") return { wave: shape(unit.units) };
    if (unit.kind === "toolGroup") return `group(${unit.tools.length})`;
    return unit.block.kind;
  });

describe("planRenderUnits", () => {
  it("folds each run of work that already has an answer after it", () => {
    const units = planRenderUnits(
      [
        reasoning(),
        toolBlock("shell"),
        text("first answer"),
        reasoning(),
        toolBlock("edit"),
        text("second answer"),
      ],
      TOOLS,
      NO_AWAITING,
    );

    expect(shape(units)).toEqual([
      { wave: ["reasoning", "tool"] },
      "text",
      { wave: ["reasoning", "tool"] },
      "text",
    ]);
  });

  it("leaves the run still in flight unfolded", () => {
    const units = planRenderUnits(
      [reasoning(), toolBlock("shell"), text("answer"), reasoning("running"), toolBlock("edit")],
      TOOLS,
      NO_AWAITING,
    );

    expect(shape(units)).toEqual([{ wave: ["reasoning", "tool"] }, "text", "reasoning", "tool"]);
  });

  it("counts a streaming answer as an answer", () => {
    const units = planRenderUnits(
      [reasoning(), toolBlock("shell"), text("part", "running")],
      TOOLS,
      NO_AWAITING,
    );

    expect(shape(units)).toEqual([{ wave: ["reasoning", "tool"] }, "text"]);
  });

  it("does not wrap a run that plans to a single row", () => {
    expect(shape(planRenderUnits([reasoning(), text("answer")], TOOLS, NO_AWAITING))).toEqual([
      "reasoning",
      "text",
    ]);
    expect(
      shape(
        planRenderUnits([toolBlock("read"), toolBlock("grep"), text("answer")], TOOLS, NO_AWAITING),
      ),
    ).toEqual(["group(2)", "text"]);
  });

  it("keeps read-only grouping inside a folded run", () => {
    const units = planRenderUnits(
      [reasoning(), toolBlock("read"), toolBlock("grep"), text("answer")],
      TOOLS,
      NO_AWAITING,
    );

    expect(shape(units)).toEqual([{ wave: ["reasoning", "group(2)"] }, "text"]);
  });

  it("folds a work-only message when its final answer follows in the next row", () => {
    const units = planRenderUnits(
      [reasoning(), toolBlock("read"), toolBlock("grep"), toolBlock("edit")],
      TOOLS,
      NO_AWAITING,
      true,
    );

    expect(shape(units)).toEqual([{ wave: ["reasoning", "group(2)", "tool"] }]);
  });

  it("never folds a block that is asking the reader for something", () => {
    const approval: ContentBlock = {
      kind: "approval",
      itemId: "item_1",
      toolName: "shell",
      command: "rm -rf build",
      reason: "destructive",
    };
    const units = planRenderUnits(
      [reasoning(), approval, toolBlock("shell"), text("done")],
      TOOLS,
      NO_AWAITING,
    );

    expect(shape(units)).toEqual(["reasoning", "approval", "tool", "text"]);
  });

  it("renders one request surface when an approval owns the same pending tool call", () => {
    const approval: ContentBlock = {
      kind: "approval",
      itemId: "shell",
      toolName: "shell",
      command: "pwd && ls",
      reason: "Runs commands in the workspace",
    };
    const tools = {
      shell: tool("shell", "shell", "exec", "running"),
    };

    expect(
      shape(planRenderUnits([toolBlock("shell"), approval], tools, new Map([["shell", "run_1"]]))),
    ).toEqual(["approval"]);
  });
});

describe("planRenderUnits · what counts as the answer", () => {
  const superseded = (blocks: ContentBlock[]) =>
    planRenderUnits(blocks, TOOLS, NO_AWAITING).map((unit) =>
      unit.kind === "wave" ? "wave" : unit.superseded,
    );

  it("keeps thinking open while the answer's block is still empty", () => {
    expect(superseded([reasoning("running"), text("", "running")])).toEqual([false, false]);
    expect(superseded([reasoning("running"), text("\n  ", "running")])).toEqual([false, false]);
  });

  it("folds thinking as soon as the answer has words", () => {
    expect(superseded([reasoning("running"), text("A", "running")])).toEqual([true, false]);
  });

  it("keeps tool work open on an empty answer block too", () => {
    expect(superseded([toolBlock("shell"), text("", "running")])).toEqual([false, false]);
    expect(superseded([toolBlock("shell"), text("done")])).toEqual([true, false]);
  });
});

describe("planRenderUnits · read-only grouping", () => {
  const tb = toolBlock;

  it("folds 2+ adjacent read-only tools into one group", () => {
    expect(planRenderUnits([tb("read"), tb("grep")], TOOLS, NO_AWAITING)).toEqual([
      { kind: "toolGroup", tools: [TOOLS.read, TOOLS.grep], superseded: false },
    ]);
  });

  it("keeps a lone read-only tool as its own block", () => {
    const blocks = [tb("read")];
    expect(planRenderUnits(blocks, TOOLS, NO_AWAITING)).toEqual([
      { kind: "block", block: blocks[0], index: 0, superseded: false },
    ]);
  });

  it("never groups side-effecting tools", () => {
    const blocks = [tb("edit"), tb("shell")];
    expect(planRenderUnits(blocks, TOOLS, NO_AWAITING)).toEqual([
      { kind: "block", block: blocks[0], index: 0, superseded: false },
      { kind: "block", block: blocks[1], index: 1, superseded: false },
    ]);
  });

  it("breaks a run on a side-effecting tool and preserves original indices", () => {
    const blocks = [tb("shell"), tb("read"), tb("grep"), tb("edit")];
    expect(planRenderUnits(blocks, TOOLS, NO_AWAITING)).toEqual([
      { kind: "block", block: blocks[0], index: 0, superseded: false },
      { kind: "toolGroup", tools: [TOOLS.read, TOOLS.grep], superseded: false },
      { kind: "block", block: blocks[3], index: 3, superseded: false },
    ]);
  });

  it("groups lsp lookups", () => {
    const tools = { one: tool("one", "lsp"), two: tool("two", "lsp") };
    expect(planRenderUnits([tb("one"), tb("two")], tools, NO_AWAITING)).toEqual([
      { kind: "toolGroup", tools: [tools.one, tools.two], superseded: false },
    ]);
  });

  it("drops a HITL-question tool's shadow row when its question block is present", () => {
    const question: ContentBlock = { kind: "question", status: "complete", questions: [] };
    const tools = { ask: tool("ask", "ask_user") };
    expect(planRenderUnits([tb("ask"), question], tools, NO_AWAITING)).toEqual([
      { kind: "block", block: question, index: 1, superseded: false },
    ]);
  });

  it("keeps a HITL-question tool row when no question block accompanies it", () => {
    const blocks = [tb("ask")];
    const tools = { ask: tool("ask", "ask_user") };
    expect(planRenderUnits(blocks, tools, NO_AWAITING)).toEqual([
      { kind: "block", block: blocks[0], index: 0, superseded: false },
    ]);
  });

  it("keeps a pending question visible without leaving its tool row behind", () => {
    const question: ContentBlock = {
      kind: "question",
      status: "complete",
      questions: [],
    };
    const tools = { ask: { ...tool("ask", "ask_user"), status: "running" as const } };

    expect(planRenderUnits([tb("ask"), question], tools, NO_AWAITING)).toEqual([
      { kind: "block", block: question, index: 1, superseded: false },
    ]);
  });

  it("retains a failed Plan approval call beside an earlier answered question", () => {
    const question: ContentBlock = { kind: "question", status: "complete", questions: [] };
    const failed = tb("exit");
    const tools = {
      exit: {
        ...tool("exit", "exit_plan_mode"),
        status: "err" as const,
        error: "current Plan is empty",
      },
    };

    expect(planRenderUnits([question, failed], tools, NO_AWAITING)).toEqual([
      { kind: "block", block: question, index: 0, superseded: false },
      { kind: "block", block: failed, index: 1, superseded: false },
    ]);
  });

  it("treats an unresolved tool block as a plain block", () => {
    const blocks = [tb("read"), tb("missing")];
    expect(planRenderUnits(blocks, TOOLS, NO_AWAITING)).toEqual([
      { kind: "block", block: blocks[0], index: 0, superseded: false },
      { kind: "block", block: blocks[1], index: 1, superseded: false },
    ]);
  });
});

describe("waveStepCount", () => {
  it("counts every step inside, thinking included and groups unpacked", () => {
    const wave = (blocks: ContentBlock[]) => {
      const first = planRenderUnits(blocks, TOOLS, NO_AWAITING)[0];
      if (first?.kind !== "wave") throw new Error("expected a folded wave");
      return waveStepCount(first.units);
    };

    expect(wave([reasoning(), toolBlock("read"), toolBlock("grep"), text("answer")])).toBe(3);
    expect(
      wave([reasoning(), toolBlock("shell"), reasoning(), toolBlock("edit"), text("answer")]),
    ).toBe(4);
  });
});
