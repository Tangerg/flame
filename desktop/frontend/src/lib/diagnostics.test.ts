import { expect, it } from "vitest";
import { failureDiagnostics } from "./diagnostics";

it("includes the error message when WebKit supplies a stack containing only frames", () => {
  const error = new Error("Reload this window to establish a clean plugin module generation.");
  error.name = "ModuleReloadRequired";
  error.stack = "component@wails://localhost/assets/index.js:42:211149";
  const diagnostics = failureDiagnostics("installed component", error);
  expect(diagnostics).toContain(`${error.name}: ${error.message}`);
  expect(diagnostics).toContain(error.stack);
});

it("preserves non-Error thrown values", () => {
  expect(failureDiagnostics("installed component", "module refused")).toContain("module refused");
});
