import { beforeEach, describe, expect, it } from "vitest";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint, subscribeContributions } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { useAppearanceStore } from "@/plugins/builtin/theme/adapters/appearanceStore";
import customTheme from "./custom-theme";

describe("custom theme contribution lifecycle", () => {
  beforeEach(() => {
    useAppearanceStore.setState({
      accent: "#3574f0",
      contrast: 25,
      customTheme: { bg: "#0f1117", fg: "#e6e8ee" },
    });
  });

  it("updates its contribution without withdrawing it when appearance preferences change", async () => {
    await loadPluginsForTest(customTheme);

    expect(lookupExtensionPoint(COLOR_THEME)).toHaveLength(1);
    expect(lookupExtensionByKey(COLOR_THEME, "custom")?.tokens?.["color-accent"]).toBe("#3574f0");
    const observed: string[][] = [];
    const unsubscribe = subscribeContributions(() => {
      observed.push(lookupExtensionPoint(COLOR_THEME).map((theme) => theme.id));
    });

    expect(() => useAppearanceStore.getState().setAccent("#7f52ff")).not.toThrow();
    expect(lookupExtensionPoint(COLOR_THEME)).toHaveLength(1);
    expect(lookupExtensionByKey(COLOR_THEME, "custom")?.tokens?.["color-accent"]).toBe("#7f52ff");
    expect(observed).toEqual([["custom"]]);
    unsubscribe();

    await resetKernelForTest();
    expect(() => useAppearanceStore.getState().setAccent("#21a179")).not.toThrow();
  });
});
