import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { ProgressBar } from "../dist/index.js";

afterEach(cleanup);

it.each([
  { value: -5, now: 0, text: "0%" },
  { value: 3, now: 3, text: "75%" },
  { value: 9, now: 4, text: "100%" },
])(
  "keeps the visible value and bounded progress in agreement for $value",
  ({ value, now, text }) => {
    render(
      <ProgressBar
        value={value}
        max={4}
        label="Review coverage"
        labelVisibility="visible"
        locale="en"
      />,
    );
    const progress = screen.getByRole("progressbar", {
      name: "Review coverage",
    });
    expect(progress.getAttribute("aria-valuenow")).toBe(String(now));
    expect(progress.getAttribute("aria-valuemax")).toBe("4");
    expect(progress.getAttribute("aria-valuetext")).toBe(text);
    expect(within(progress).getByText(text)).toBeTruthy();
    expect(within(progress).getByText("Review coverage")).toBeTruthy();
  },
);

it("keeps compact application progress named without duplicating its surrounding caption", () => {
  const view = render(
    <ProgressBar value={75} label="Build workspace" weight="seam" />,
  );
  const progress = screen.getByRole("progressbar", { name: "Build workspace" });
  expect(progress.getAttribute("aria-valuemax")).toBe("100");
  expect(progress.getAttribute("aria-valuenow")).toBe("75");
  expect(screen.queryByText("Build workspace")).toBeNull();
  expect(screen.queryByText("75%")).toBeNull();
  view.rerender(
    <ProgressBar value={100} label="Build workspace" weight="seam" />,
  );
  expect(screen.getByRole("progressbar", { name: "Build workspace" })).toBe(
    progress,
  );
  expect(progress.getAttribute("aria-valuenow")).toBe("100");
});
