import { beforeEach, describe, expect, it } from "vitest";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/extensions/selectors/extensions";
import { subscribeContributions } from "@/extensions/kernel";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { solidFill } from "@/features/theme/kit/scale";
import customTheme from "@/features/theme/themes/custom-theme";
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
    expect(lookupExtensionByKey(COLOR_THEME, "custom")?.tokens?.["color-accent"]).toBe(
      solidFill("#3574f0"),
    );
    const observed: string[][] = [];
    const unsubscribe = subscribeContributions(() => {
      observed.push(lookupExtensionPoint(COLOR_THEME).map((theme) => theme.id));
    });

    expect(() => useAppearanceStore.getState().setAccent("#7f52ff")).not.toThrow();
    expect(lookupExtensionPoint(COLOR_THEME)).toHaveLength(1);
    expect(lookupExtensionByKey(COLOR_THEME, "custom")?.tokens?.["color-accent"]).toBe(
      solidFill("#7f52ff"),
    );
    expect(observed).toEqual([["custom"]]);
    unsubscribe();

    await resetKernelForTest();
    expect(() => useAppearanceStore.getState().setAccent("#21a179")).not.toThrow();
  });
});
