import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n/index";
import { type ToolCall } from "@/features/agent/public/sessionView";
import {
  toolDiffStat,
  isReadOnlyTool,
  summarizeActivity,
  toolActivityStatus,
  toolGroupNeedsAttention,
  toolIntent,
  toolMetaItems,
} from "@/features/agent/presentation/toolPresentation";
import { toolCategoryForName } from "@/test/toolCategory.generated";

// The Runtime stamps a category on built-in calls only; fixtures reproduce that.
const tool = ({ runId = "run_1", ...overrides }: Partial<ToolCall>): ToolCall => {
  const category = toolCategoryForName(overrides.name ?? "shell");
  return {
    id: "tool-1",
    runId,
    name: "shell",
    fn: "shell",
    args: "",
    status: "ok",
    ...(category !== undefined ? { category } : {}),
    ...overrides,
  };
};

describe("toolPresentation", () => {
  it("projects tool args into a compact intent", () => {
    expect(
      toolIntent(
        t,
        tool({ name: "read", fn: "read", args: JSON.stringify({ path: "src/App.tsx" }) }),
      ),
    ).toEqual({
      label: { kind: "prose", value: "Read" },
      detail: { kind: "path", value: "src/App.tsx" },
    });
  });

  it("names another owner's tool by itself and keeps its searched object", () => {
    expect(
      toolIntent(
        t,
        tool({
          name: "web_search",
          fn: "web_search",
          args: JSON.stringify({ query: "sqlite wal checkpoint starvation" }),
        }),
      ),
    ).toEqual({
      label: { kind: "machine", value: "web_search" },
      detail: { kind: "machine", value: "sqlite wal checkpoint starvation" },
    });
  });

  it("drops a detail that repeats the label", () => {
    expect(toolIntent(t, tool({ name: "shell", fn: "pnpm test", command: "pnpm test" }))).toEqual({
      label: { kind: "prose", value: "Ran" },
      detail: { kind: "machine", value: "pnpm test" },
    });
    expect(
      toolIntent(t, tool({ name: "shell", fn: "Run the unit tests", command: "pnpm test" })),
    ).toEqual({
      label: { kind: "prose", value: "Run the unit tests" },
      detail: { kind: "machine", value: "pnpm test" },
    });
  });

  it("says so when the target is a path", () => {
    expect(
      toolIntent(t, tool({ name: "apply_patch", fn: "runtime/store.go", fnKind: "path" })),
    ).toEqual({
      label: { kind: "prose", value: "Applied patch" },
      detail: { kind: "path", value: "runtime/store.go" },
    });
  });

  it("marks a pattern as machine text, not as a path", () => {
    expect(
      toolIntent(t, tool({ name: "grep", fn: "grep", args: JSON.stringify({ pattern: "a/b" }) }))
        .detail,
    ).toEqual({ kind: "machine", value: "a/b" });
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

  it.each(["err", "denied"] as const)(
    "does not claim that an unaccepted Plan update succeeded (%s)",
    (status) => {
      const call = tool({ name: "set_plan", fn: "set_plan", status });
      expect(toolIntent(t, call)).toEqual({
        label: { kind: "prose", value: "Update plan" },
        detail: undefined,
      });
    },
  );

  it("names a tool it has no verb for by its own name", () => {
    expect(toolIntent(t, tool({ name: "acme_docs", fn: "acme_docs" }))).toEqual({
      label: { kind: "machine", value: "acme_docs" },
    });
  });

  it("ignores malformed args while keeping the tool label", () => {
    expect(toolIntent(t, tool({ name: "acme_docs", fn: "acme_docs", args: "{" }))).toEqual({
      label: { kind: "machine", value: "acme_docs" },
    });
  });

  describe("captured snapshot offset", () => {
    const browse = (args: string, overrides: Partial<ToolCall> = {}) =>
      toolIntent(t, tool({ name: "web_web_browse", fn: "web_web_browse", args, ...overrides }));
    const label = { kind: "machine", value: "web_web_browse" };

    it("names the requested pair, never the result's offsets", () => {
      expect(
        browse(`{"snapshotId":"snap-7f3a","offset":8192,"limit":128}`, {
          result: JSON.stringify({ offset: 8320, nextOffset: 8448, content: "…" }),
        }),
      ).toEqual({
        label,
        detail: { kind: "machine", value: `{"offset":8192,"snapshotId":"snap-7f3a"}` },
      });
    });

    it.each([0, -128])("keeps an explicit %d offset literal", (offset) => {
      expect(browse(`{"snapshotId":"snap-7f3a","offset":${offset}}`).detail).toEqual({
        kind: "machine",
        value: `{"offset":${offset},"snapshotId":"snap-7f3a"}`,
      });
    });

    it.each([
      ["an omitted offset", `{"snapshotId":"snap-7f3a","limit":128}`],
      ["an omitted snapshot", `{"offset":8192}`],
      ["an empty snapshot", `{"snapshotId":"","offset":8192}`],
      ["a numeric snapshot", `{"snapshotId":7,"offset":8192}`],
      ["a null offset", `{"snapshotId":"snap-7f3a","offset":null}`],
      ["a string offset", `{"snapshotId":"snap-7f3a","offset":"8192"}`],
      ["a fractional offset", `{"snapshotId":"snap-7f3a","offset":81.92}`],
      ["a non-canonical offset", `{"snapshotId":"snap-7f3a","offset":8192.0}`],
      ["a negative-zero offset", `{"snapshotId":"snap-7f3a","offset":-0}`],
      ["an unsafe offset", `{"snapshotId":"snap-7f3a","offset":9007199254740993}`],
    ])("adds no detail for %s", (_, args) => {
      expect(browse(args)).toEqual({ label });
    });

    it("leaves an existing object or explicit target primary", () => {
      const pair = `"snapshotId":"snap-7f3a","offset":8192`;
      expect(browse(`{${pair},"url":"https://example.com/a"}`).detail).toEqual({
        kind: "machine",
        value: "https://example.com/a",
      });
      expect(browse(`{${pair},"path":"docs/a.md"}`).detail).toEqual({
        kind: "path",
        value: "docs/a.md",
      });
      expect(browse(`{${pair}}`, { fn: "https://example.com/b" }).detail).toEqual({
        kind: "machine",
        value: "https://example.com/b",
      });
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

  it("counts pattern hits as matches", () => {
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
    expect(toolDiffStat(tool({ added: 4 }))).toEqual({ added: 4, removed: undefined });
    expect(toolDiffStat(tool({ added: 0, removed: 0 }))).toBeUndefined();
    expect(toolDiffStat(tool({}))).toBeUndefined();
  });

  it("preserves an unrecorded side beside a known change count", () => {
    expect(toolDiffStat(tool({ added: 4 }))).toEqual({ added: 4, removed: undefined });
    expect(toolDiffStat(tool({ removed: 3 }))).toEqual({ added: undefined, removed: 3 });
    expect(toolDiffStat(tool({ added: 0 }))).toEqual({ added: 0, removed: undefined });
    expect(toolDiffStat(tool({ removed: 0 }))).toEqual({ added: undefined, removed: 0 });
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

  describe("Skill page offset", () => {
    const snapshot = "0f".repeat(32);
    const skillArgs = (cursor?: string) =>
      cursor === undefined ? `{"name":"review-diff"}` : `{"name":"review-diff","cursor":${cursor}}`;
    const page = (offset: number) =>
      JSON.stringify({ content: "…", next_cursor: { offset, snapshot } });

    it("names the incoming byte offset after duration, never the result's next cursor", () => {
      const continued = tool({
        name: "load_skill",
        fn: "review-diff",
        args: skillArgs(`{"offset":134,"snapshot":"${snapshot}"}`),
        result: page(266),
        durationMillis: 1200,
      });
      expect(toolMetaItems(t, continued)).toEqual([
        { id: "duration", label: "1.2s", tone: "muted" },
        { id: "offset", label: "from byte 134", tone: "muted" },
      ]);
    });

    it("leaves a call that starts a page sequence as it was", () => {
      const initial = tool({
        name: "load_skill",
        fn: "review-diff",
        args: skillArgs(),
        result: page(134),
      });
      expect(toolMetaItems(t, initial)).toEqual([]);
    });

    it("names a continued Skill resource read", () => {
      const resource = tool({
        name: "read_skill_resource",
        fn: "review-diff/checklist.md",
        args: JSON.stringify({
          name: "review-diff",
          path: "checklist.md",
          cursor: { offset: 81, snapshot },
        }),
      });
      expect(toolMetaItems(t, resource)).toEqual([
        { id: "offset", label: "from byte 81", tone: "muted" },
      ]);
    });

    it.each([
      ["malformed arguments", `{"name":"review-diff","cursor":{"offset":134`],
      ["a null cursor", skillArgs("null")],
      ["an array cursor", skillArgs(`[134,"${snapshot}"]`)],
      ["a missing snapshot", skillArgs(`{"offset":134}`)],
      ["a short snapshot", skillArgs(`{"offset":134,"snapshot":"${snapshot.slice(1)}"}`)],
      ["a non-hex snapshot", skillArgs(`{"offset":134,"snapshot":"${"zz".repeat(32)}"}`)],
      ["a zero offset", skillArgs(`{"offset":0,"snapshot":"${snapshot}"}`)],
      ["a negative offset", skillArgs(`{"offset":-134,"snapshot":"${snapshot}"}`)],
      ["a fractional offset", skillArgs(`{"offset":13.4,"snapshot":"${snapshot}"}`)],
      ["a non-canonical offset", skillArgs(`{"offset":134.0,"snapshot":"${snapshot}"}`)],
      ["an unsafe offset", skillArgs(`{"offset":9007199254740993,"snapshot":"${snapshot}"}`)],
      ["a string offset", skillArgs(`{"offset":"134","snapshot":"${snapshot}"}`)],
    ])("adds no offset for %s", (_, args) => {
      expect(toolMetaItems(t, tool({ name: "load_skill", fn: "review-diff", args }))).toEqual([]);
    });

    it("keeps byte wording to the Runtime's Skill text pages", () => {
      const args = JSON.stringify({ name: "review-diff", cursor: { offset: 134, snapshot } });
      expect(toolMetaItems(t, tool({ name: "list_skills", fn: "list_skills", args }))).toEqual([]);
      expect(
        toolMetaItems(t, tool({ name: "load_skill", fn: "load_skill", category: undefined, args })),
      ).toEqual([]);
      expect(
        toolMetaItems(t, tool({ name: "acme_load_skill", fn: "acme_load_skill", args })),
      ).toEqual([]);
    });
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
      tool({ id: "codeintel_lsp", name: "codeintel_lsp" }),
    ];
    expect(summarizeActivity(t, tools)).toBe("1 read · 3 search");
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

  it("counts settled problems once and distinguishes denials and calls still in flight", () => {
    expect(
      toolActivityStatus([
        tool({ status: "ok", exitCode: 0 }),
        tool({ status: "ok", exitCode: 1 }),
        tool({ status: "err", exitCode: 2 }),
        tool({ status: "err" }),
        tool({ status: "denied", exitCode: 1 }),
        tool({ status: "running", exitCode: 1 }),
      ]),
    ).toEqual({ running: 1, issues: 3, denied: 1 });
    expect(toolActivityStatus([])).toEqual({ running: 0, issues: 0, denied: 0 });
  });
});
