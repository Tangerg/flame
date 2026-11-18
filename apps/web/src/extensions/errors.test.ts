import { describe, expect, it } from "vitest";
import { reportPluginError, usePluginErrorStore } from "@/extensions/errors";
describe("usePluginErrorStore", () => {
  it("clearFor removes only the given plugin's entries", () => {
    const { push, clearFor } = usePluginErrorStore.getState();
    push({ plugin: "a", source: "setup", message: "1" });
    push({ plugin: "b", source: "setup", message: "2" });
    push({ plugin: "a", source: "render", message: "3" });

    clearFor("a");

    const remaining = usePluginErrorStore.getState().log;
    expect(remaining).toHaveLength(1);
    expect(remaining[0]!.plugin).toBe("b");
  });
});

describe("reportPluginError", () => {
  it("retains the original cause and every failed cleanup in terminal diagnostics", () => {
    const failure = new Error("host quarantined", {
      cause: new AggregateError(
        [new Error("original operation failed"), new Error("resource release was unconfirmed")],
        "retirement failed",
      ),
    });
    reportPluginError("p", "events", failure);
    const detail = usePluginErrorStore.getState().log[0]!.detail;
    expect(detail).toContain("original operation failed");
    expect(detail).toContain("resource release was unconfirmed");
  });
});
