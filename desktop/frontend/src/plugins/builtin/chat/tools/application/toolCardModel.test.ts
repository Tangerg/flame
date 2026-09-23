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
  it("keeps the subject of a failed call and reports the failure beside it", () => {
    const model = toolCardModel(
      t,
      tool({ status: "err", error: "permission denied", command: "rm -rf /" }),
    );

    expect(model.detail).toMatchObject({ value: "rm -rf /" });
    expect(model.error).toBe("permission denied");
  });

  it("withholds a path detail that would name one file out of several", () => {
    const one = toolCardModel(
      t,
      tool({ name: "apply_patch", fn: "a.ts", fnKind: "path", files: 1 }),
    );
    const many = toolCardModel(
      t,
      tool({ name: "apply_patch", fn: "a.ts", fnKind: "path", files: 3 }),
    );

    expect(one.detail).toMatchObject({ kind: "path", value: "a.ts" });
    expect(many.detail).toBeUndefined();
    expect(many.metaItems.some((item) => item.id === "files")).toBe(true);
  });

  it("keeps a detail that is not one of the files", () => {
    const grep = toolCardModel(t, tool({ name: "grep", fn: "TODO", files: 3 }));

    expect(grep.detail).toMatchObject({ value: "TODO" });
  });

  it("carries no failure for a call that did not fail", () => {
    expect(toolCardModel(t, tool({ status: "ok" })).error).toBeUndefined();
    expect(toolCardModel(t, tool({ status: "denied", error: "refused" })).error).toBeUndefined();
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
