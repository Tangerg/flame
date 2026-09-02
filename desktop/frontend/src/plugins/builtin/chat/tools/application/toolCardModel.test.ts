import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import type { ToolActionSpec, ToolViewOpenerSpec } from "@/plugins/sdk";
import {
  headlineToolMetaItem,
  toolCardActions,
  toolCardModel,
  toolCardViewOpener,
} from "./toolCardModel";

const tool = ({ runId = "run_1", ...overrides }: Partial<ToolCall> = {}): ToolCall => ({
  id: "tool-1",
  runId,
  name: "shell",
  fn: "shell",
  args: "go test ./...",
  status: "ok",
  safetyClass: "exec",
  ...overrides,
});

describe("toolCardModel", () => {
  it("lets an error message own the collapsed detail line", () => {
    expect(
      toolCardModel(
        t,
        tool({
          status: "err",
          error: "permission denied",
          args: '{"cmd":"rm"}',
        }),
      ),
    ).toMatchObject({ detail: { kind: "text", value: "permission denied" } });
  });

  it("projects lifecycle flags and presentation data", () => {
    const model = toolCardModel(t, tool({ status: "requires-action" }));

    expect(model).toMatchObject({ running: false, denied: false });
    expect(model.intent.label).toBeTruthy();
    expect(Array.isArray(model.metaItems)).toBe(true);
  });

  it("tells a refused call apart from a finished one", () => {
    expect(toolCardModel(t, tool({ status: "denied" })).denied).toBe(true);
    expect(toolCardModel(t, tool({ status: "ok" })).denied).toBe(false);
  });
});

describe("toolCardActions", () => {
  it("keeps actions with no predicate or a matching predicate", () => {
    const actions: ToolActionSpec[] = [
      { id: "always", icon: "copy", title: "Always", run: () => undefined },
      {
        id: "shell",
        icon: "terminal",
        title: "Shell",
        predicate: (candidate) => candidate.name === "shell",
        run: () => undefined,
      },
      {
        id: "read",
        icon: "file",
        title: "Read",
        predicate: (candidate) => candidate.name === "read",
        run: () => undefined,
      },
    ];

    expect(toolCardActions(tool({ name: "shell" }), actions).map((action) => action.id)).toEqual([
      "always",
      "shell",
    ]);
  });
});

describe("toolCardViewOpener", () => {
  it("selects the first opener whose predicate matches the tool", () => {
    const openers: ToolViewOpenerSpec[] = [
      { id: "read", predicate: (candidate) => candidate.name === "read", open: () => undefined },
      { id: "shell", predicate: (candidate) => candidate.name === "shell", open: () => undefined },
    ];

    expect(toolCardViewOpener(tool({ name: "shell" }), openers)?.id).toBe("shell");
  });
});

describe("headlineToolMetaItem", () => {
  // The compact row has one slot. It used to take whatever sat last in the list, so a call
  // that exited non-zero showed its duration instead — in the faint tone, at that.
  it("gives the one slot to a failure over a measurement", () => {
    const items = [
      { id: "hits", label: "3 matches", tone: "muted" },
      { id: "exit", label: "exit 1", tone: "negative" },
      { id: "duration", label: "4.2s", tone: "muted" },
    ] as const;

    expect(headlineToolMetaItem(items)?.id).toBe("exit");
    expect(headlineToolMetaItem(items.filter((item) => item.id !== "exit"))?.id).toBe("duration");
    expect(headlineToolMetaItem([])).toBeUndefined();
  });
});
