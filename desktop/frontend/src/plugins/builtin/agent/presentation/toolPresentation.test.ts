import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import {
  toolDiffStat,
  isReadOnlyTool,
  summarizeActivity,
  toolGroupNeedsAttention,
  toolIntent,
  toolMetaItems,
} from "./toolPresentation";

const tool = ({ runId = "run_1", ...overrides }: Partial<ToolCall>): ToolCall => ({
  id: "tool-1",
  runId,
  name: "shell",
  fn: "shell",
  args: "",
  status: "ok",
  ...overrides,
});

describe("toolPresentation", () => {
  it("projects tool args into a compact intent", () => {
    expect(
      toolIntent(
        t,
        tool({ name: "read", fn: "read", args: JSON.stringify({ path: "src/App.tsx" }) }),
      ),
    ).toEqual({
      label: { kind: "text", value: "Read" },
      detail: { kind: "path", value: "src/App.tsx" },
    });
  });

  it("drops a detail that repeats the label", () => {
    expect(toolIntent(t, tool({ name: "shell", fn: "pnpm test", command: "pnpm test" }))).toEqual({
      label: { kind: "text", value: "Ran" },
      detail: { kind: "text", value: "pnpm test" },
    });
    expect(
      toolIntent(t, tool({ name: "shell", fn: "Run the unit tests", command: "pnpm test" })),
    ).toEqual({
      label: { kind: "text", value: "Run the unit tests" },
      detail: { kind: "text", value: "pnpm test" },
    });
  });

  it("says so when the target is a path", () => {
    expect(
      toolIntent(t, tool({ name: "apply_patch", fn: "runtime/store.go", fnKind: "path" })),
    ).toEqual({
      label: { kind: "text", value: "Applied patch" },
      detail: { kind: "path", value: "runtime/store.go" },
    });
  });

  it("marks a pattern as text, not as a path", () => {
    expect(
      toolIntent(t, tool({ name: "grep", fn: "grep", args: JSON.stringify({ pattern: "a/b" }) }))
        .detail,
    ).toEqual({ kind: "text", value: "a/b" });
  });

  it("keeps a command verbatim even when it reads like a tool name", () => {
    expect(toolIntent(t, tool({ name: "shell", fn: "grep" })).detail?.value).toBe("grep");
    expect(toolIntent(t, tool({ name: "shell", fn: "shell" })).label.value).toBe("Ran");
  });

  it("words a call in flight as ongoing and a settled one as finished", () => {
    const reading = tool({ name: "read", fn: "src/App.tsx", fnKind: "path", status: "running" });
    expect(toolIntent(t, reading).label.value).toBe("Reading");
    expect(toolIntent(t, { ...reading, status: "ok" }).label.value).toBe("Read");
    expect(toolIntent(t, { ...reading, status: "denied" }).label.value).toBe("Read file");
    expect(toolIntent(t, { ...reading, status: "err" }).label.value).toBe("Read file");
  });

  it.each(["err", "denied", "requires-action"] as const)(
    "does not claim that an unaccepted Plan update succeeded (%s)",
    (status) => {
      const call = tool({ name: "set_plan", fn: "set_plan", status });
      expect(toolIntent(t, call)).toEqual({
        label: { kind: "text", value: "Update plan" },
        detail: undefined,
      });
    },
  );

  it("gives a tool it has no verb for the generic one", () => {
    expect(toolIntent(t, tool({ name: "acme_docs", fn: "acme_docs" }))).toEqual({
      label: { kind: "text", value: "Used tool" },
      detail: { kind: "text", value: "acme_docs" },
    });
  });

  it("ignores malformed args while keeping the tool label", () => {
    expect(toolIntent(t, tool({ name: "acme_docs", fn: "acme_docs", args: "{" }))).toEqual({
      label: { kind: "text", value: "Used tool" },
      detail: { kind: "text", value: "acme_docs" },
    });
  });

  it("derives ordered meta badges", () => {
    expect(
      toolMetaItems(t, tool({ added: 3, removed: 2, hits: 7, exitCode: 1, status: "running" })),
    ).toEqual([
      { id: "hits", label: "7 matches", tone: "muted" },
      { id: "exit", label: "exit 1", tone: "negative" },
    ]);
  });

  it("counts web search results as results and pattern hits as matches", () => {
    expect(toolMetaItems(t, tool({ name: "web_search", fn: "web_search", hits: 2 }))).toEqual([
      { id: "hits", label: "2 results", tone: "muted" },
    ]);
    expect(toolMetaItems(t, tool({ name: "grep", fn: "grep", hits: 2 }))).toEqual([
      { id: "hits", label: "2 matches", tone: "muted" },
    ]);
  });

  it("reports a partial read's span as notation", () => {
    expect(toolMetaItems(t, tool({ range: { start: 40, end: 80 }, lines: 900 }))).toEqual([
      { id: "range", label: "L40-80", tone: "muted" },
      { id: "lines", label: "900 lines", tone: "muted" },
    ]);
  });

  it("reports a diffstat only when it has something to say", () => {
    expect(toolDiffStat(tool({ added: 3, removed: 2 }))).toEqual({ added: 3, removed: 2 });
    expect(toolDiffStat(tool({ added: 4 }))).toEqual({ added: 4, removed: 0 });
    expect(toolDiffStat(tool({ added: 0, removed: 0 }))).toBeUndefined();
    expect(toolDiffStat(tool({}))).toBeUndefined();
  });

  it("does not dress a refused or failed call in the size of the change it proposed", () => {
    expect(toolDiffStat(tool({ added: 3, removed: 2, status: "denied" }))).toBeUndefined();
    expect(toolDiffStat(tool({ added: 3, removed: 2, status: "err" }))).toBeUndefined();
    expect(toolDiffStat(tool({ added: 3, removed: 2, status: "running" }))).toEqual({
      added: 3,
      removed: 2,
    });
  });

  it("reports a measured duration, and only once it is worth reading", () => {
    expect(toolMetaItems(t, tool({ durationMillis: 4200 })).map((item) => item.id)).toEqual([
      "duration",
    ]);
    expect(toolMetaItems(t, tool({ durationMillis: 120 }))).toEqual([]);
    expect(toolMetaItems(t, tool({}))).toEqual([]);
  });

  it("takes read-only from the runtime's safety class", () => {
    expect(isReadOnlyTool(tool({ name: "read", safetyClass: "safe" }))).toBe(true);
    expect(isReadOnlyTool(tool({ name: "apply_patch", safetyClass: "write" }))).toBe(false);
    expect(isReadOnlyTool(tool({ name: "acme_do_thing" }))).toBe(false);
  });

  it("summarizes grouped tools by display bucket", () => {
    const tools = [
      tool({ id: "read", name: "read" }),
      tool({ id: "grep", name: "grep" }),
      tool({ id: "glob", name: "glob" }),
      tool({ id: "lsp", name: "lsp" }),
    ];
    expect(summarizeActivity(t, tools)).toBe("1 read · 2 search · 1 lookup");
  });

  it("tells acts apart by the runtime's safety class", () => {
    const tools = [
      tool({ id: "read", name: "read", safetyClass: "safe" }),
      tool({ id: "patch", name: "apply_patch", safetyClass: "write" }),
      tool({ id: "sh", name: "shell", safetyClass: "exec" }),
      tool({ id: "web", name: "web_fetch", safetyClass: "network" }),
    ];
    expect(summarizeActivity(t, tools)).toBe("1 read · 1 write · 1 run · 1 fetch");
  });

  it("keeps a fixed family order regardless of call order", () => {
    const reversed = [
      tool({ id: "sh", name: "shell", safetyClass: "exec" }),
      tool({ id: "read", name: "read", safetyClass: "safe" }),
    ];
    expect(summarizeActivity(t, reversed)).toBe("1 read · 1 run");
  });

  it("has nothing to summarize for a round that only thought", () => {
    expect(summarizeActivity(t, [])).toBe("");
  });

  it("marks groups needing attention only while running or failed", () => {
    expect(toolGroupNeedsAttention([tool({ status: "ok" })])).toBe(false);
    expect(toolGroupNeedsAttention([tool({ status: "running" })])).toBe(true);
    expect(toolGroupNeedsAttention([tool({ status: "err" })])).toBe(true);
  });
});
