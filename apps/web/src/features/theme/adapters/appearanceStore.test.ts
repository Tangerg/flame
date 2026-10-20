import { describe, expect, it } from "vitest";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
describe("appearance defaults", () => {
  it("starts on the same theme the pre-paint bootstrap and the native frame assume", () => {
    expect(useAppearanceStore.getInitialState().theme).toBe("system");
  });

  it("starts on Flame Orange as the default primary color", () => {
    expect(useAppearanceStore.getInitialState().accent).toBe("#fb6e30");
  });
});
