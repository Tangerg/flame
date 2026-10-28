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

describe("appearance preference persistence", () => {
  it.each(["codeFontSize", "paperTexture", "paperTextureStrength"] as const)(
    "refuses an incomplete appearance record missing %s",
    async (missing) => {
      const warning = vi.spyOn(console, "warn").mockImplementation(() => {});
      const state: Partial<AppearancePreference> = {
        ...store.getInitialState(),
        theme: "custom",
        accent: "#7f52ff",
        customTheme: { bg: "#f2ede4", fg: "#202020" },
        uiFont: "Georgia",
        fontSize: 18,
        codeFontSize: 16,
        motionScale: 0,
      };
      delete state[missing];
      localStorage.setItem("flame.appearance", JSON.stringify({ state, version: 0 }));
      await store.persist.rehydrate();
      expect(warning).toHaveBeenCalledOnce();
      expect(store.getState()).toEqual(store.getInitialState());
    },
  );

  it("restores complete current preferences from the actual storage adapter", async () => {
    store.getState().setTheme("custom");
    store.getState().setAccent("#7f52ff");
    store.getState().setCodeFontSize(16);
    store.getState().setPaperTexture("cotton");
    store.getState().setPaperTextureStrength(5);
    const retained = store.getState();
    const saved = localStorage.getItem("flame.appearance")!;
    store.setState(store.getInitialState());
    localStorage.setItem("flame.appearance", saved);
    await store.persist.rehydrate();
    expect(store.getState()).toEqual(retained);
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
