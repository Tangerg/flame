import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { createAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { type AppearancePreference } from "@/features/theme/kit/appearance";

let store: ReturnType<typeof createAppearanceStore>;
beforeEach(() => {
  store = createAppearanceStore(() => localStorage);
});
afterEach(() => {
  localStorage.clear();
  store.setState(store.getInitialState());
  vi.restoreAllMocks();
});
describe("appearance defaults", () => {
  it("starts on the same theme the pre-paint bootstrap and the native frame assume", () => {
    expect(store.getInitialState().theme).toBe("system");
  });

  it("starts on Flame Orange as the default primary color", () => {
    expect(store.getInitialState().accent).toBe("#fb6e30");
  });

  it("starts without a paper texture", () => {
    expect(store.getInitialState().paperTexture).toBe("off");
    expect(store.getInitialState().paperTextureStrength).toBe(3);
  });
});

describe("paper preference persistence", () => {
  it("retains existing appearance choices when the new paper fields are absent", async () => {
    const state: Partial<AppearancePreference> = { ...store.getInitialState() };
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
    await store.persist.rehydrate();
    expect(store.getState()).toMatchObject({
      ...state,
      paperTexture: "off",
      paperTextureStrength: 3,
    });
  });

  it("restores the selected paper and strength from the actual storage adapter", async () => {
    store.getState().setPaperTexture("cotton");
    store.getState().setPaperTextureStrength(5);
    const saved = localStorage.getItem("flame.appearance")!;
    store.setState(store.getInitialState());
    localStorage.setItem("flame.appearance", saved);
    await store.persist.rehydrate();
    expect(store.getState()).toMatchObject({
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
    const state = { ...store.getInitialState(), ...invalid };
    localStorage.setItem("flame.appearance", JSON.stringify({ state, version: 0 }));
    await store.persist.rehydrate();
    expect(warning).toHaveBeenCalledOnce();
    expect(store.getState()).toMatchObject({
      paperTexture: "off",
      paperTextureStrength: 3,
    });
  });
});
