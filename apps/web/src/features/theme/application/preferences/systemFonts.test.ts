import { describe, expect, it } from "vitest";
import {
  systemFonts,
  type FontAvailability,
} from "@/features/theme/application/preferences/systemFonts";
function fakePort(installed: string[], tabular: string[]): FontAvailability {
  return {
    isAvailable: (family) => installed.includes(family),
    hasTabularFigures: (family) => tabular.includes(family),
  };
}

describe("systemFonts", () => {
  it("offers only families that are installed", () => {
    const availability = fakePort(["Helvetica Neue", "Arial"], ["Helvetica Neue", "Arial"]);
    const result = systemFonts(availability, false);
    expect(result).toEqual(["Helvetica Neue", "Arial"]);
  });

  it("drops a family that cannot render tabular figures", () => {
    const availability = fakePort(["Helvetica Neue", "Arial"], ["Helvetica Neue"]);
    const result = systemFonts(availability, false);
    expect(result).toEqual(["Helvetica Neue"]);
  });

  it("never offers a missing family, whatever the tabular answer says", () => {
    const availability = fakePort([], ["Arial", "Roboto", "Inter"]);
    const result = systemFonts(availability, false);
    expect(result).toEqual([]);
  });

  it("asks the same two questions of code families", () => {
    const availability = fakePort(["Menlo", "Monaco"], ["Menlo"]);
    const result = systemFonts(availability, true);
    expect(result).toEqual(["Menlo"]);
  });

  it("returns an empty list rather than inventing a family", () => {
    const availability = fakePort([], []);
    expect(systemFonts(availability, false)).toEqual([]);
    expect(systemFonts(availability, true)).toEqual([]);
  });
});
