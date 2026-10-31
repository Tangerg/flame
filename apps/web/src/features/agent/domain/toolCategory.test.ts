import { describe, expect, it } from "vitest";
import { builtinToolName, isQuestionTool } from "@/features/agent/domain/toolCategory";

describe("isQuestionTool", () => {
  it("knows the two built-in tools that interrupt from inside their own call", () => {
    expect(isQuestionTool({ name: "ask_user", category: "task" })).toBe(true);
    expect(isQuestionTool({ name: "exit_plan_mode", category: "task" })).toBe(true);
    expect(isQuestionTool({ name: "shell", category: "shell" })).toBe(false);
    expect(isQuestionTool({ name: "constructor", category: "task" })).toBe(false);
  });

  it("never reads a plugin or MCP tool as a built-in of the same name", () => {
    expect(isQuestionTool({ name: "ask_user" })).toBe(false);
    expect(builtinToolName({ name: "read" })).toBeUndefined();
    expect(builtinToolName({ name: "read", category: "read" })).toBe("read");
  });
});
