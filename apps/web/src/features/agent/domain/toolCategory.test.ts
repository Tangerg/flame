import { describe, expect, it } from "vitest";
import { isQuestionTool } from "@/features/agent/domain/toolCategory";

describe("isQuestionTool", () => {
  it("knows the two tools that interrupt from inside their own call", () => {
    expect(isQuestionTool("ask_user")).toBe(true);
    expect(isQuestionTool("exit_plan_mode")).toBe(true);
    expect(isQuestionTool("shell")).toBe(false);
    expect(isQuestionTool("constructor")).toBe(false);
  });
});
