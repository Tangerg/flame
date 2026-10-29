import { parseJSON } from "@qimu/sdk/json";
import { type ToolInvocation } from "@flame/runtime-client";
import { describe, expect, it } from "vitest";
import {
  argsText,
  toolFields,
  toolLabel,
  toolLabelKind,
} from "@/features/agent/application/fold/projections";
import { toolCategoryForName } from "@/features/agent/application/fold/reducer.fixtures";
const tool = (name: string, args: Record<string, unknown>, result?: unknown): ToolInvocation =>
  ({ name, category: toolCategoryForName(name), arguments: args, result }) as ToolInvocation;

it("renders original opaque Tool arguments and output without numeric conversion", () => {
  const value = parseJSON('{"wide":9007199254740993,"decimal":1.0}') as Record<string, unknown>;
  const invocation = tool("independent_tool", value, value);
  for (const text of [argsText(invocation), toolFields(invocation).result]) {
    expect(text).toContain("9007199254740993");
    expect(text).toContain("1.0");
  }
});

describe("toolLabel — name-keyed specialised tools", () => {
  it("lsp position operations label path:line:character", () => {
    expect(
      toolLabel(
        tool("codeintel_lsp", { operation: "definition", path: "main.go", line: 42, character: 3 }),
      ),
    ).toBe("main.go:42:3");
    expect(
      toolLabel(tool("codeintel_lsp", { operation: "hover", path: "a.ts", line: 1, character: 5 })),
    ).toBe("a.ts:1:5");
  });

  it("lsp file/query operations label by their key argument", () => {
    expect(
      toolLabel(tool("codeintel_lsp", { operation: "document_symbols", path: "main.go" })),
    ).toBe("main.go");
    expect(
      toolLabel(tool("codeintel_lsp", { operation: "workspace_symbols", query: "ReadTool" })),
    ).toBe("ReadTool");
    expect(toolLabel(tool("codeintel_lsp", { operation: "diagnostics", path: "main.go" }))).toBe(
      "main.go",
    );
  });

  it("the Skill family labels by name; ask_user labels the first question", () => {
    expect(toolLabel(tool("load_skill", { name: "review" }))).toBe("review");
    expect(toolLabel(tool("propose_skill", { name: "review-go-api" }))).toBe("review-go-api");
    expect(toolLabel(tool("read_skill_resource", { name: "review", path: "checklist.md" }))).toBe(
      "review/checklist.md",
    );
    expect(
      toolLabel(tool("ask_user", { questions: [{ question: "Deploy now?\ndetails…" }] })),
    ).toBe("Deploy now?");
  });

  it("delegate_task (subagent category) labels its summary", () => {
    expect(
      toolLabel(tool("delegate_task", { summary: "Audit the retry path", instructions: "…" })),
    ).toBe("Audit the retry path");
  });

  it("the searches and fetches label by query and url", () => {
    expect(toolLabel(tool("search_memory", { query: "deploy runbook" }))).toBe("deploy runbook");
    expect(toolLabel(tool("search_tools", { query: "screenshot" }))).toBe("screenshot");
    expect(toolLabel(tool("web_fetch", { url: "https://go.dev" }))).toBe("https://go.dev");
    expect(toolLabel(tool("web_http_request", { url: "https://api.local/health" }))).toBe(
      "https://api.local/health",
    );
  });

  it("shell labels its description, and the shell pollers their shell id", () => {
    expect(
      toolLabel(tool("shell", { command: "npm run dev", description: "Start the dev server" })),
    ).toBe("Start the dev server");
    expect(toolLabel(tool("read_shell_output", { shell_id: "bg_1" }))).toBe("bg_1");
    expect(toolLabel(tool("stop_shell", { shell_id: "bg_2" }))).toBe("bg_2");
  });

  it("identifies stored-output reads by result id without treating the id as a file path", () => {
    for (const id of ["BLOB234", "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567".repeat(2)]) {
      const invocation = tool("read_tool_result", {
        result_id: id,
        offset_bytes: 4000,
        limit_bytes: 1024,
      });

      expect(toolLabel(invocation)).toBe(id);
      expect(toolLabelKind(invocation)).toBe("text");
    }
  });
});

describe("toolFields — runtime wire shapes", () => {
  it("delegate_task: projects the completed reply as text and preserves failure results", () => {
    const reply = '## Findings\n\nThe "child" completed.\n\n末尾。';
    expect(toolFields(tool("delegate_task", {}, { reply })).result).toBe(reply);
    expect(toolFields(tool("delegate_task", {}))).not.toHaveProperty("result");
    expect(toolFields(tool("delegate_task", {}, "error: delegated worker canceled")).result).toBe(
      "error: delegated worker canceled",
    );
  });

  it("shell: reads the projected {output, exitCode} and carries the command", () => {
    const f = toolFields(
      tool(
        "shell",
        { command: "go test", description: "Run the tests" },
        { output: "ok\nwarn", exitCode: 1 },
      ),
    );
    expect(f.result).toBe("ok\nwarn");
    expect(f.exitCode).toBe(1);
    expect(f.command).toBe("go test");
  });

  it("shell: a backgrounded call's plain-string ack passes through with no exit code", () => {
    const f = toolFields(
      tool(
        "shell",
        { command: "npm run dev", run_in_background: true },
        "Started background shell bg_1.",
      ),
    );
    expect(f.result).toBe("Started background shell bg_1.");
    expect(f.exitCode).toBeUndefined();
  });

  it("grep: hits come from the runtime's single projected envelope", () => {
    expect(toolFields(tool("grep", {}, { hits: [{}, {}] })).hits).toBe(2);
    expect(toolFields(tool("grep", {}, { hits: ["a", "b", "c"] })).hits).toBe(3);
    expect(toolFields(tool("glob", {}, { hits: [{ path: "x" }] })).hits).toBe(1);
  });

  it("apply_patch: shows no diffstat when the receipt names no change", () => {
    const empty = toolFields(tool("apply_patch", { patch: "…" }, { files: [] }));
    expect(empty.added).toBeUndefined();
    expect(empty.removed).toBeUndefined();
    expect(empty.changes).toBeUndefined();
    const absent = toolFields(tool("apply_patch", { patch: "…" }));
    expect(absent.added).toBeUndefined();
    expect(absent.changes).toBeUndefined();
  });

  it("apply_patch: projects the runtime's change receipt, counts and all", () => {
    const f = toolFields(
      tool(
        "apply_patch",
        { patch: "…" },
        {
          changes: [{ path: "src/a.ts", status: "modified", added: 2, removed: 1 }],
        },
      ),
    );

    expect(f.added).toBe(2);
    expect(f.removed).toBe(1);
    expect(f.changes).toEqual([{ path: "src/a.ts", status: "modified", added: 2, removed: 1 }]);
  });

  it("apply_patch: carries the authoritative call-scoped change receipt", () => {
    // The argument is a non-git envelope the client cannot parse; it reads the
    // runtime's receipt instead, so the diffstat and file count still hold.
    const f = toolFields(
      tool(
        "apply_patch",
        { patch: "*** Begin Patch\n…" },
        {
          changes: [
            { path: "src/new.ts", status: "added", added: 4, removed: 0 },
            { path: "src/current.ts", status: "moved", from: "src/old.ts", added: 1, removed: 1 },
          ],
        },
      ),
    );

    expect(f.added).toBe(5);
    expect(f.removed).toBe(1);
    expect(f.files).toBe(2);
    expect(JSON.parse(f.result ?? "null")).toEqual({
      changes: [
        { path: "src/new.ts", status: "added", added: 4, removed: 0 },
        { path: "src/current.ts", status: "moved", from: "src/old.ts", added: 1, removed: 1 },
      ],
    });
  });

  it("read: passes the content through and reports how long the file is", () => {
    expect(
      toolFields(tool("read", { path: "a.go" }, { content: "package main", total_lines: 412 })),
    ).toMatchObject({ result: "package main", lines: 412 });
    expect(toolFields(tool("read", { path: "a.go" }, { content: "x" })).lines).toBeUndefined();
    expect(
      toolFields(
        tool(
          "read",
          { path: "a.go" },
          { content: "first\nlast", total_lines: 100, start_line: 11, end_line: 12 },
        ),
      ),
    ).toEqual({ result: "first\nlast", lines: 100, range: { start: 11, end: 12 } });
  });

  it.each(["read_skill_resource", "read_tool_result"])(
    "%s retains literal source instead of interpreting a file-result envelope",
    (name) => {
      for (const output of [
        "[recorded source]\n\n\tKeep <component> tags literal.\n",
        '{"content":"keep this nested value","total_lines":9007199254740993}',
        "",
      ]) {
        expect(toolFields(tool(name, {}, output))).toEqual({ result: output });
      }
      expect(toolFields(tool(name, {}))).toEqual({});
    },
  );

  it("retains a Skill resource page's continuation alongside its captured content", () => {
    const page = {
      content: "Keep this exact text.\n\n\tNext paragraph.",
      next_cursor: { offset: 39, snapshot: "a".repeat(64) },
    };
    const fields = toolFields(tool("read_skill_resource", { name: "review", path: "a.md" }, page));

    expect(JSON.parse(fields.result!)).toEqual(page);
    expect(fields.result).toBe(JSON.stringify(page, null, 2));
    expect(fields.lines).toBeUndefined();
    expect(fields.range).toBeUndefined();
  });
});

describe("argsText — complete recorded arguments remain separate from summaries", () => {
  it.each([
    ["codeintel_lsp", { operation: "definition", path: "a.go", line: 1, character: 1 }],
    ["load_skill", { name: "review", cursor: { offset: 134, snapshot: "a".repeat(64) } }],
    [
      "read_skill_resource",
      { name: "review", path: "a.md", cursor: { offset: 81, snapshot: "b".repeat(64) } },
    ],
    ["read", { path: "a.go", offset: 12, limit: 8 }],
    ["read_tool_result", { result_id: "BLOB234", offset: 20, limit: 40 }],
    ["shell", { command: "go test ./...", timeout: 10 }],
    ["web_http_request", { url: "https://example.invalid", method: "POST", body: "literal" }],
    ["linear_create_issue", { title: "t" }],
  ] as const)("retains every captured parameter for %s", (name, args) => {
    expect(JSON.parse(argsText(tool(name, args)))).toEqual(args);
  });

  it("does not manufacture arguments for an empty call", () => {
    expect(argsText(tool("list_skills", {}))).toBe("");
  });
});

it("preserves rejected argument text without interpreting it as executable arguments", () => {
  expect(argsText({ name: "read", arguments: {}, argumentsText: '{"path":' })).toBe('{"path":');
});
