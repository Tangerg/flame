import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ComponentBoundary } from "@/features/plugins/ui/InstalledComponent";
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

it("keeps reload behind the original cleanup gate and exposes an unconfirmed exit", async () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const closing = Promise.withResolvers<void>();
  const reload = vi.fn(() => closing.promise);
  function Broken(): never {
    throw new Error("native render failed");
  }
  testClient.render(
    <ComponentBoundary scope="installed component" reload={reload}>
      <Broken />
    </ComponentBoundary>,
  );
  const button = screen.getByRole("button", { name: "Reload window" }) as HTMLButtonElement;
  fireEvent.click(button);
  expect(button.disabled).toBe(true);
  expect(reload).toHaveBeenCalledOnce();
  closing.reject(new Error("original resource release remains unconfirmed"));
  await waitFor(() => expect(button.disabled).toBe(false));
  const details = screen.getAllByRole("button", { name: "Details" });
  fireEvent.click(details[0]!);
  expect(screen.getByText(/scope: installed component reload/).textContent).toContain(
    "original resource release remains unconfirmed",
  );
});

it("retains the original component exception for inspection before reloading", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const error = new Error("schedule render evidence");
  function Broken(): never {
    throw error;
  }
  testClient.render(
    <ComponentBoundary scope="installed component">
      <Broken />
    </ComponentBoundary>,
  );
  expect(screen.getByRole("alert")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Details" }));
  expect(screen.getByText(/scope: installed component/).textContent).toContain(error.stack);
});

it("captures component identity, generation, time and React stack at the failure", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-17T00:00:00Z"));
  const scope = `installed component\ninstallation: schedule-installation\nrelease: ${"a".repeat(64)}\ncomponent: schedules\nruntime generation: original-process`;
  function BrokenSchedule(): never {
    throw new Error("original schedule exception");
  }
  const view = testClient.render(
    <ComponentBoundary scope={scope}>
      <BrokenSchedule />
    </ComponentBoundary>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Details" }));
  const diagnostics = screen.getByText(/scope: installed component/).textContent;
  expect(diagnostics).toContain("installation: schedule-installation");
  expect(diagnostics).toContain(`release: ${"a".repeat(64)}`);
  expect(diagnostics).toContain("component: schedules");
  expect(diagnostics).toContain("runtime generation: original-process");
  expect(diagnostics).toContain("time: 2026-10-17T00:00:00.000Z");
  expect(diagnostics).toContain("react component stack:");
  expect(diagnostics).toContain("BrokenSchedule");
  vi.setSystemTime(new Date("2026-10-17T01:00:00Z"));
  view.rerender(
    <ComponentBoundary scope="replacement component">
      <BrokenSchedule />
    </ComponentBoundary>,
  );
  expect(screen.getByText(/scope: installed component/).textContent).toBe(diagnostics);
});
