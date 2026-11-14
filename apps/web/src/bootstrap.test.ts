import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import { startupCopy } from "@/app/startupCopy.generated";
import { LOCALE_STORAGE_KEY } from "@/lib/i18n/locale";

let root: HTMLElement;
let previousLanguage: string;
beforeEach(() => {
  vi.resetModules();
  root = document.createElement("div");
  root.id = "root";
  root.textContent = "Opening Flame";
  document.body.append(root);
  previousLanguage = document.documentElement.lang;
  localStorage.setItem(LOCALE_STORAGE_KEY, "zh");
  vi.spyOn(HTMLScriptElement, "supports").mockReturnValue(true);
  vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(async () => {
  await vi.dynamicImportSettled();
  vi.doUnmock("./main");
  vi.resetModules();
  vi.restoreAllMocks();
  root.remove();
  document.documentElement.lang = previousLanguage;
  localStorage.removeItem(LOCALE_STORAGE_KEY);
});

it("renders a localized capability diagnosis without importing the main module", async () => {
  const evaluateMain = vi.fn(() => ({}));
  vi.doMock("./main", evaluateMain);
  vi.mocked(HTMLScriptElement.supports).mockReturnValue(false);
  await import("./bootstrap");
  await vi.dynamicImportSettled();
  const alert = screen.getByRole("alert", { name: startupCopy.zh["startup.failed.title"] });
  expect(alert.textContent).toContain(startupCopy.zh["startup.unsupported.body"]);
  expect(alert.textContent).toContain('HTMLScriptElement.supports("importmap")');
  expect(evaluateMain).not.toHaveBeenCalled();
  expect(console.error).not.toHaveBeenCalled();
});

it("imports an admitted main module once and leaves its rendering owner in control", async () => {
  const evaluateMain = vi.fn(() => ({}));
  vi.doMock("./main", evaluateMain);
  await import("./bootstrap");
  await vi.dynamicImportSettled();
  expect(evaluateMain).toHaveBeenCalledOnce();
  expect(root.textContent).toBe("Opening Flame");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(console.error).not.toHaveBeenCalled();
});

it("keeps an import rejection in local diagnostics and renders a localized failure", async () => {
  const original = new Error("<script>dependency failed</script>", {
    cause: new Error("dependency root cause"),
  });
  const evaluateMain = vi.fn(() => {
    throw original;
  });
  vi.doMock("./main", evaluateMain);
  await import("./bootstrap");
  await vi.dynamicImportSettled();
  expect(evaluateMain).toHaveBeenCalledOnce();
  expect(console.error).toHaveBeenCalledExactlyOnceWith(
    "[desktop] application import failed:",
    expect.any(Error),
  );
  const reported = vi.mocked(console.error).mock.calls[0]![1] as Error;
  // Vitest wraps a failing module factory and retains its original error as the cause.
  expect(reported.cause).toBe(original);
  expect(original.cause).toBeInstanceOf(Error);
  const alert = screen.getByRole("alert", { name: startupCopy.zh["startup.failed.title"] });
  expect(alert.textContent).toContain(startupCopy.zh["startup.loadFailed.body"]);
  expect(
    within(alert).getByRole("region", { name: startupCopy.zh["plugins.renderFailed.details"] })
      .textContent,
  ).toBe(reported.message);
  expect(within(alert).getByRole("button", { name: startupCopy.zh["common.retry"] })).toBeTruthy();
  expect(root.querySelector("script")).toBeNull();
});
