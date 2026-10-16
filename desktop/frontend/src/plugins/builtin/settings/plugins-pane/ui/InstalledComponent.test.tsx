import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ComponentBoundary } from "./InstalledComponent";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

it("retains the original component exception for inspection before reloading", () => {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const error = new Error("schedule render evidence");
  function Broken(): never {
    throw error;
  }
  render(
    <ComponentBoundary>
      <Broken />
    </ComponentBoundary>,
  );
  expect(screen.getByRole("alert")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Details" }));
  expect(screen.getByText(/scope: installed component/).textContent).toContain(error.stack);
});
