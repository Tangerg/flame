import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { configureFontAvailabilityPort } from "./ports/fontAvailability";
import { useSystemFonts } from "./systemFonts";

function fakePort(installed: string[], tabular: string[]) {
  return configureFontAvailabilityPort({
    isAvailable: (family) => installed.includes(family),
    hasTabularFigures: (family) => tabular.includes(family),
  });
}

let restore: (() => void) | undefined;
afterEach(() => {
  restore?.();
  restore = undefined;
});

describe("useSystemFonts", () => {
  it("offers only families that are installed", () => {
    restore = fakePort(["Helvetica Neue", "Arial"], ["Helvetica Neue", "Arial"]);
    const { result } = renderHook(() => useSystemFonts(false));
    expect(result.current).toEqual(["Helvetica Neue", "Arial"]);
  });

  it("drops a family that cannot render tabular figures", () => {
    restore = fakePort(["Helvetica Neue", "Arial"], ["Helvetica Neue"]);
    const { result } = renderHook(() => useSystemFonts(false));
    expect(result.current).toEqual(["Helvetica Neue"]);
  });

  it("never offers a missing family, whatever the tabular answer says", () => {
    restore = fakePort([], ["Arial", "Roboto", "Inter"]);
    const { result } = renderHook(() => useSystemFonts(false));
    expect(result.current).toEqual([]);
  });

  it("asks the same two questions of code families", () => {
    restore = fakePort(["Menlo", "Monaco"], ["Menlo"]);
    const { result } = renderHook(() => useSystemFonts(true));
    expect(result.current).toEqual(["Menlo"]);
  });

  it("returns an empty list rather than inventing a family", () => {
    restore = fakePort([], []);
    expect(renderHook(() => useSystemFonts(false)).result.current).toEqual([]);
    expect(renderHook(() => useSystemFonts(true)).result.current).toEqual([]);
  });
});
