import { describe, expect, it } from "vitest";
import { isQuestionTool, toolCategory } from "./toolCategory";

describe("toolCategory", () => {
  it("names the runtime's own tools", () => {
    expect(toolCategory("shell")).toBe("command");
    expect(toolCategory("apply_patch")).toBe("fileEdit");
    expect(toolCategory("grep")).toBe("search");
    expect(toolCategory("glob")).toBe("search");
    expect(toolCategory("delegate_task")).toBe("subagent");
  });

  it("leaves everything else generic", () => {
    expect(toolCategory("weather_lookup")).toBe("generic");
    expect(toolCategory("")).toBe("generic");
  });

  // An MCP server names its own tools, so this is indexed by a string nobody in this
  // app chose. An object-literal table answered these from `Object.prototype` — a
  // FUNCTION, typed as ToolCategory, which is neither a category nor the fallback.
  it("leaves a tool named after an inherited member generic", () => {
    for (const name of ["constructor", "toString", "valueOf", "hasOwnProperty", "__proto__"]) {
      expect(toolCategory(name)).toBe("generic");
    }
  });
});

describe("isQuestionTool", () => {
  it("knows the two tools that interrupt from inside their own call", () => {
    expect(isQuestionTool("ask_user")).toBe(true);
    expect(isQuestionTool("exit_plan_mode")).toBe(true);
    expect(isQuestionTool("shell")).toBe(false);
    expect(isQuestionTool("constructor")).toBe(false);
  });
});
