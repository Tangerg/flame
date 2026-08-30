import { describe, expect, it } from "vitest";
import type { AgentToolInvocation } from "@/plugins/sdk";
import { TOOL_FAMILIES } from "@/lib/toolFamilies";
import { Arbitrary, forEachSeed } from "@/test/arbitrary";
import {
  argsText,
  commandString,
  editableArgs,
  toolFields,
  toolLabel,
  toolLabelKind,
} from "./projections";

// `arguments` and `result` are `unknown` by contract: the Runtime forwards whatever a tool
// or a model produced, and an MCP server's tool is not ours at all. These projections run on
// EVERY tool call, inside the fold — one throw does not spoil a card, it aborts the reducer
// and the transcript stops advancing. So the property is totality, not any particular label.

const TOOL_NAMES = [
  ...TOOL_FAMILIES.flatMap((family) => family.tools.map((tool) => tool.name)),
  "mcp__linear__create_issue",
  "",
  "UNKNOWN_TOOL",
];

function hostileValue(a: Arbitrary, depth = 0): unknown {
  const pick = a.int(depth > 2 ? 6 : 10);
  switch (pick) {
    case 0:
      return a.text();
    case 1:
      return a.int(1000);
    case 2:
      return a.bool();
    case 3:
      return null;
    case 4:
      return undefined;
    case 5:
      return a.text().repeat(3);
    case 6:
      return [hostileValue(a, depth + 1), hostileValue(a, depth + 1)];
    case 7:
      return { [a.text()]: hostileValue(a, depth + 1) };
    case 8:
      return Object.assign(Object.create(null), { question: hostileValue(a, depth + 1) });
    default:
      return { nested: { deeper: hostileValue(a, depth + 1) } };
  }
}

// The keys these projections actually reach for, so the fuzz lands on the branches that
// matter rather than only on the default arm.
const READ_KEYS = [
  "command",
  "description",
  "path",
  "operation",
  "query",
  "line",
  "character",
  "shell_id",
  "name",
  "url",
  "questions",
  "summary",
  "output",
  "exitCode",
  "content",
  "changes",
  "pattern",
];

function hostileTool(a: Arbitrary): AgentToolInvocation {
  const args: Record<string, unknown> = {};
  for (const key of READ_KEYS) if (a.bool(0.45)) args[key] = hostileValue(a);
  if (a.bool(0.3)) args[a.text()] = hostileValue(a);
  return {
    name: a.pick(TOOL_NAMES),
    arguments: args,
    ...(a.bool(0.7) ? { result: hostileValue(a) } : {}),
  };
}

describe("the tool projections, over the arguments a tool can actually carry", () => {
  it("reaches every family, so the properties below are not fuzzing one branch", () => {
    const seen = new Set<string>();
    forEachSeed(400, (a) => seen.add(hostileTool(a).name));
    expect(seen.size).toBeGreaterThan(20);
  });

  it("never throws, whatever the arguments hold", () => {
    const thrown: string[] = [];
    forEachSeed(600, (a) => {
      const tool = hostileTool(a);
      for (const project of [
        toolLabel,
        toolLabelKind,
        toolFields,
        argsText,
        commandString,
        editableArgs,
      ]) {
        try {
          project(tool);
        } catch (error) {
          thrown.push(`${project.name}(${tool.name}): ${String(error)}`);
        }
      }
    });
    expect(thrown.slice(0, 5)).toEqual([]);
  });

  it("answers a label that can go straight into a row", () => {
    forEachSeed(600, (a) => {
      const tool = hostileTool(a);
      const label = toolLabel(tool);
      expect(typeof label).toBe("string");
      // A row truncates from the left when it is a path. Calling a sentence a path puts the
      // ellipsis on the wrong end, so the kind must stay one of exactly two answers.
      expect(["path", "text"]).toContain(toolLabelKind(tool));
      expect(label).not.toContain("\n");
    });
  });

  it("answers a command only when the argument really was one", () => {
    const fabricated: string[] = [];
    forEachSeed(400, (a) => {
      const tool = hostileTool(a);
      const command = commandString(tool);
      expect(typeof command).toBe("string");
      if (typeof tool.arguments.command !== "string" && command !== "") {
        fabricated.push(`${tool.name}: ${command}`);
      }
    });
    expect(fabricated).toEqual([]);
  });

  it("answers args text and editable args in their declared shapes", () => {
    forEachSeed(400, (a) => {
      const tool = hostileTool(a);
      expect(typeof argsText(tool)).toBe("string");
      const editable = editableArgs(tool);
      expect(editable === undefined || typeof editable === "object").toBe(true);
    });
  });

  it("never invents a field the projection is not meant to own", () => {
    forEachSeed(400, (a) => {
      const fields = toolFields(hostileTool(a));
      expect(typeof fields).toBe("object");
      expect(Array.isArray(fields)).toBe(false);
      for (const value of Object.values(fields)) expect(value).not.toBeNaN();
    });
  });
});
