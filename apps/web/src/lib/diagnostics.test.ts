import { expect, it } from "vitest";
import { failureDiagnostics, failureMessage } from "@/lib/diagnostics";
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

it("retains module and cleanup causes under the boundary's failure context", () => {
  const original = new Error("publisher module exception");
  const cleanup = new Error("resource retirement failed");
  const error = new AggregateError(
    [new Error("component evaluation failed", { cause: original }), cleanup],
    "plugin loading failed",
  );
  const diagnostics = failureDiagnostics("installed component", error);
  expect(diagnostics).toContain("scope: installed component");
  expect(diagnostics).toContain(original.stack);
  expect(diagnostics).toContain(cleanup.stack);
});

it("bounds circular error references while preserving the original exception", () => {
  const error = new Error("original exception");
  error.cause = error;
  const diagnostics = failureDiagnostics("installed component", error);
  expect(diagnostics).toContain(error.stack);
  expect(diagnostics).toContain("[circular error reference]");
});

it("keeps the original resource failure in an aggregate lifecycle message", () => {
  const cleanup = new Error("stream close failed");
  const error = new AggregateError([cleanup], "Lifetime has unresolved cleanup obligations");
  error.errors.push(error);
  expect(failureMessage(error)).toBe(
    "Lifetime has unresolved cleanup obligations; stream close failed",
  );
});
