import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { configureFontAvailabilityPort } from "./ports/fontAvailability";
import { useSystemFonts } from "./systemFonts";

// What the picker offers is two questions, and both of them used to be answered wrong.
//
// "Is it installed" was `document.fonts.check()`, which answers `true` for a family that does
// not exist — measured saying so for all nine UI candidates AND for a made-up name, on a
// machine carrying two of them. The filter it backed did nothing.
//
// "Can it keep the promise" was not asked at all, so `Arial` was offered while moving a
// ten-digit run 21.4px, against a design that says numbers do not jitter.
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

  // Order matters and is not cosmetic: asking whether a MISSING family has tabular figures
  // measures whatever the fallback is, so a family absent from the machine must never reach
  // the second question and must never be offered on the strength of its answer.
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

  // An empty list is a usable control rather than a broken one: the picker's own default entry
  // resolves to the native system stack, which is what the product runs on regardless.
  it("returns an empty list rather than inventing a family", () => {
    restore = fakePort([], []);
    expect(renderHook(() => useSystemFonts(false)).result.current).toEqual([]);
    expect(renderHook(() => useSystemFonts(true)).result.current).toEqual([]);
  });
});
