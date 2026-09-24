import { describe, expect, it } from "vitest";
import { useAppearanceStore } from "./appearanceStore";

describe("appearance defaults", () => {
  it("starts on the same theme the pre-paint bootstrap and the native frame assume", () => {
    expect(useAppearanceStore.getInitialState().theme).toBe("system");
  });
});
