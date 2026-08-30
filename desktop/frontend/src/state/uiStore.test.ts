import { beforeEach, describe, expect, it, vi } from "vitest";
import { useUiStore } from "./uiStore";

// The accent is not stored as an opaque string: the theme decomposes it into OKLCH
// to derive the neutral family, and `parseInt(hex, 16)` reads whatever prefix parses
// rather than rejecting a non-hex value. "blue" comes back as a finite garbage
// colour and every derived surface paints black, so a payload carrying one has to
// be refused at the boundary rather than believed.
const STORAGE_KEY = "flame.ui";
const DEFAULTS = { ...useUiStore.getState() };

function bootWith(state: Record<string, unknown>) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({ state, version: 14 }));
  useUiStore.persist.rehydrate();
}

beforeEach(() => {
  localStorage.clear();
  // `merge` falls back to what the store currently holds, so each case has to start
  // from the pristine defaults or it measures the previous case's accent.
  useUiStore.setState(DEFAULTS);
  vi.spyOn(console, "warn").mockImplementation(() => {});
});

describe("a persisted appearance payload", () => {
  it("keeps a well-formed accent", () => {
    bootWith({ ...DEFAULTS, accent: "#ff8800" });
    expect(useUiStore.getState().accent).toBe("#ff8800");
  });

  it("refuses an accent that is not a colour and boots on the default", () => {
    bootWith({ ...DEFAULTS, accent: "blue" });
    expect(useUiStore.getState().accent).toBe(DEFAULTS.accent);
  });

  it.each(["", "#fff", "#12345g", "rgb(0,0,0)"])("refuses the accent %o", (accent) => {
    bootWith({ ...DEFAULTS, accent });
    expect(useUiStore.getState().accent).toBe(DEFAULTS.accent);
  });

  it("refuses a custom theme colour that is not a colour", () => {
    bootWith({ ...DEFAULTS, customTheme: { bg: "black", fg: "#e6e8ee" } });
    expect(useUiStore.getState().customTheme.bg).toBe(DEFAULTS.customTheme.bg);
  });
});
