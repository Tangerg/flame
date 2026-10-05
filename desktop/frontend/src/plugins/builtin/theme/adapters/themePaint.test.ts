import { readFileSync } from "node:fs";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { COLOR_THEME } from "@/plugins/sdk/kernelPoints";
import { contributeForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { rememberThemePaint } from "./themePaint";
import { installDocumentAppearance } from "./documentAppearance";
import { installAppearancePreferencePort } from "./appearancePreferenceBinding";
import { useAppearanceStore } from "./appearanceStore";
import { configureSystemAppearancePort } from "../application/ports/systemAppearance";
import { resolveThemeScheme, retainThemeSelection } from "../application/themeScheme";

const theme = "package:installation:light";
const html = readFileSync("index.html", "utf8");

beforeEach(() => {
  localStorage.clear();
  document.documentElement.className = "";
  document.documentElement.removeAttribute("style");
  installAppearancePreferencePort();
  configureSystemAppearancePort({ scheme: () => "dark" });
});

afterEach(async () => {
  await resetKernelForTest();
});

it("uses the rendered projection for first paint without admitting a runtime theme", () => {
  rememberThemePaint(theme, "light", "#fafafa");
  localStorage.setItem("flame.appearance", JSON.stringify({ state: { theme } }));
  const bootstrap = html.match(/<script>([\s\S]*?)<\/script>/)?.[1];
  expect(bootstrap).toBeDefined();
  Function(bootstrap!)();
  expect(document.documentElement.classList.contains("theme-light")).toBe(true);
  expect(document.documentElement.style.backgroundColor).toBe("#fafafa");
  expect(resolveThemeScheme(theme)).toBe("dark");
});

it.each([
  [false, "theme-light"],
  [true, "theme-dark"],
])(
  "first-paints an unpainted theme in the system scheme (dark system: %s)",
  (systemDark, schemeClass) => {
    vi.stubGlobal("matchMedia", (query: string) => ({ matches: systemDark, media: query }));
    try {
      localStorage.setItem("flame.appearance", JSON.stringify({ state: { theme } }));
      Function(html.match(/<script>([\s\S]*?)<\/script>/)![1]!)();
      expect(document.documentElement.className).toBe(schemeClass);
    } finally {
      vi.unstubAllGlobals();
    }
  },
);

it("cannot advance the selected preference through a paint projection", () => {
  rememberThemePaint(theme, "light", "#fafafa");
  expect(useAppearanceStore.getState().theme).not.toBe(theme);
  useAppearanceStore.setState({ theme });
  retainThemeSelection([theme], "package:");
  expect(useAppearanceStore.getState().theme).toBe(theme);
  retainThemeSelection([], "package:");
  expect(useAppearanceStore.getState().theme).toBe("system");
});

it("refuses malformed paint data and ignores another source's selection", () => {
  localStorage.setItem(
    "flame.theme-paint",
    JSON.stringify({ theme, scheme: "light", background: "url(https://example.test)" }),
  );
  localStorage.setItem("flame.appearance", JSON.stringify({ state: { theme } }));
  const bootstrap = html.match(/<script>([\s\S]*?)<\/script>/)?.[1];
  Function(bootstrap!)();
  expect(document.documentElement.style.backgroundColor).not.toContain("url(");
  useAppearanceStore.setState({ theme: "light" });
  retainThemeSelection([], "package:");
  expect(useAppearanceStore.getState().theme).toBe("light");
});

it("forgets the paint of a package theme Runtime stops admitting so cold boot follows the system", async () => {
  const uninstall = installDocumentAppearance(useAppearanceStore);
  try {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, { id: "dark", label: "Dark", scheme: "dark" });
      ctx.contribute(COLOR_THEME, {
        id: theme,
        label: "Package",
        scheme: "light",
        tokens: { "color-bg": "#fafafa" },
      });
    });
    useAppearanceStore.setState({ theme });
    expect(localStorage.getItem("flame.theme-paint")).toContain(`"theme":"${theme}"`);

    retainThemeSelection([], "package:");

    expect(useAppearanceStore.getState().theme).toBe("system");
    expect(localStorage.getItem("flame.theme-paint")).toBeNull();
    document.documentElement.className = "";
    document.documentElement.removeAttribute("style");
    vi.stubGlobal("matchMedia", (query: string) => ({ matches: true, media: query }));
    try {
      Function(html.match(/<script>([\s\S]*?)<\/script>/)![1]!)();
    } finally {
      vi.unstubAllGlobals();
    }
    expect(document.documentElement.className).toBe("theme-dark");
    expect(document.documentElement.style.backgroundColor).not.toBe("rgb(250, 250, 250)");
  } finally {
    uninstall();
  }
});
