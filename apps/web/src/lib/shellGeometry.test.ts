import { expect, it } from "vitest";
import { canPresentDock } from "./shellGeometry";
it("requires room for both the reading surface and dock", () => {
  expect(canPresentDock(759)).toBe(false);
  expect(canPresentDock(760)).toBe(true);
});
