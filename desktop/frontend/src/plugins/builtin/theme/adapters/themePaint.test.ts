import { readFileSync } from "node:fs";
import { beforeEach, expect, it } from "vitest";
import { rememberThemePaint } from "./themePaint";
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
