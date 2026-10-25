import { beforeEach, expect, it } from "vitest";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { contributeForTest } from "@/test/kernel";
import { installAppearancePreferencePort } from "@/features/theme/adapters/appearancePreferenceBinding";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { solidFill } from "@/features/theme/kit/scale";
import { contributePaletteTheme } from "@/features/theme/application/paletteTheme";
beforeEach(() => {
  installAppearancePreferencePort();
  useAppearanceStore.setState({
    accent: "#3574f0",
    contrast: 25,
    paperTexture: "off",
    paperTextureStrength: 3,
  });
});

it("updates a contributed palette for paper and strength, then restores its plain inks", async () => {
  await contributeForTest((ctx) => {
    contributePaletteTheme(ctx.lifetime("paper"), {
      id: "package:paper",
      label: "Paper",
      scheme: "light",
      palette: {},
    });
  });
  const tokens = () => lookupExtensionByKey(COLOR_THEME, "package:paper")?.tokens;
  const plain = tokens();
  useAppearanceStore.getState().setPaperTexture("xuan");
  const medium = tokens();
  expect(medium?.["color-text-faint"]).not.toBe(plain?.["color-text-faint"]);
  useAppearanceStore.getState().setPaperTextureStrength(5);
  expect(tokens()?.["color-text-faint"]).not.toBe(medium?.["color-text-faint"]);
  useAppearanceStore.getState().setPaperTexture("off");
  expect(tokens()).toEqual(plain);
});

it("re-derives a palette theme from the current accent and contrast", async () => {
  await contributeForTest((ctx) => {
    contributePaletteTheme(ctx.lifetime("palette"), {
      id: "package:theme",
      label: "Package",
      scheme: "dark",
      palette: { background: "#101418", foreground: "#e6e8ee" },
    });
  });
  const tokens = () => lookupExtensionByKey(COLOR_THEME, "package:theme")?.tokens;
  const surface = tokens()?.["color-surface"];

  useAppearanceStore.setState({ contrast: 90, accent: "#7f52ff" });

  expect(tokens()?.["color-surface"]).not.toBe(surface);
  expect(tokens()?.["color-accent"]).toBe(solidFill("#7f52ff"));
});
