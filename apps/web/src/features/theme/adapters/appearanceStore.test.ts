import { afterEach, describe, expect, it, vi } from "vitest";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { type AppearancePreference } from "@/features/theme/kit/appearance";

afterEach(() => {
  localStorage.clear();
  useAppearanceStore.setState(useAppearanceStore.getInitialState());
  vi.restoreAllMocks();
});
describe("appearance defaults", () => {
  it("starts on the same theme the pre-paint bootstrap and the native frame assume", () => {
    expect(useAppearanceStore.getInitialState().theme).toBe("system");
  });

  it("starts on Flame Orange as the default primary color", () => {
    expect(useAppearanceStore.getInitialState().accent).toBe("#fb6e30");
  });

  it("starts without a paper texture", () => {
    expect(useAppearanceStore.getInitialState().paperTexture).toBe("off");
    expect(useAppearanceStore.getInitialState().paperTextureStrength).toBe(3);
  });
});

describe("paper preference persistence", () => {
  it("retains existing appearance choices when the new paper fields are absent", async () => {
    const state: Partial<AppearancePreference> = { ...useAppearanceStore.getInitialState() };
    delete state.paperTexture;
    delete state.paperTextureStrength;
    Object.assign(state, {
      theme: "custom",
      accent: "#7f52ff",
      customTheme: { bg: "#f2ede4", fg: "#202020" },
      uiFont: "Georgia",
      fontSize: 18,
      codeFontSize: 16,
      motionScale: 0,
    });
    localStorage.setItem("flame.appearance", JSON.stringify({ state, version: 0 }));
    await useAppearanceStore.persist.rehydrate();
    expect(useAppearanceStore.getState()).toMatchObject({
      ...state,
      paperTexture: "off",
      paperTextureStrength: 3,
    });
  });

  it("restores the selected paper and strength from the actual storage adapter", async () => {
    useAppearanceStore.getState().setPaperTexture("cotton");
    useAppearanceStore.getState().setPaperTextureStrength(5);
    const saved = localStorage.getItem("flame.appearance")!;
    useAppearanceStore.setState(useAppearanceStore.getInitialState());
    localStorage.setItem("flame.appearance", saved);
    await useAppearanceStore.persist.rehydrate();
    expect(useAppearanceStore.getState()).toMatchObject({
      paperTexture: "cotton",
      paperTextureStrength: 5,
    });
  });

  it.each([
    { paperTexture: "unknown" },
    { paperTextureStrength: 0 },
    { paperTextureStrength: 6 },
    { paperTextureStrength: 2.5 },
  ])("rejects a malformed paper preference (%j)", async (invalid) => {
    const warning = vi.spyOn(console, "warn").mockImplementation(() => {});
    const state = { ...useAppearanceStore.getInitialState(), ...invalid };
    localStorage.setItem("flame.appearance", JSON.stringify({ state, version: 0 }));
    await useAppearanceStore.persist.rehydrate();
    expect(warning).toHaveBeenCalledOnce();
    expect(useAppearanceStore.getState()).toMatchObject({
      paperTexture: "off",
      paperTextureStrength: 3,
    });
  });
});
