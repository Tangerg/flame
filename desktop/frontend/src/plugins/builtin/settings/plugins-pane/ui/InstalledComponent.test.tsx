import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ComponentBoundary } from "./InstalledComponent";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

it("retains the original component exception for inspection before reloading", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const error = new Error("schedule render evidence");
  function Broken(): never {
    throw error;
  }
  render(
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
  const view = render(
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
