import { beforeEach, expect, it } from "vitest";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey } from "@/plugins/sdk/selectors/extensions";
import { contributeForTest } from "@/plugins/sdk/testKernel";
import { installAppearancePreferencePort } from "../adapters/appearancePreferenceBinding";
import { useAppearanceStore } from "../adapters/appearanceStore";
import { contributePaletteTheme } from "./paletteTheme";

beforeEach(() => {
  installAppearancePreferencePort();
  useAppearanceStore.setState({ accent: "#3574f0", contrast: 25 });
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
  expect(tokens()?.["color-accent"]).toBe("#7f52ff");
});
