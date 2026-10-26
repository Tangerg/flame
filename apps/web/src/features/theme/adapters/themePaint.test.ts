import { createTheme, type Theme } from "@/features/theme/adapters/theme";
let owner: Theme;
import { readFileSync } from "node:fs";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { COLOR_THEME } from "@/features/theme/public/extensionPoints";
import { contributeForTest, resetKernelForTest } from "@/test/kernel";
import { rememberThemePaint } from "@/features/theme/adapters/themePaint";
import { resolveThemeScheme, retainThemeSelection } from "@/features/theme/application/themeScheme";
const theme = "package:installation:light";

const html = readFileSync("index.html", "utf8");

beforeEach(() => {
  localStorage.clear();
  document.documentElement.className = "";
  document.documentElement.removeAttribute("style");
  owner = createTheme(document, window);
  vi.spyOn(owner.access.system, "scheme").mockReturnValue("dark");
});

afterEach(async () => {
  owner.dispose();
  await resetKernelForTest();
});

it("uses the rendered projection for first paint without admitting a runtime theme", () => {
  rememberThemePaint(() => localStorage, theme, "light", "#fafafa");
  localStorage.setItem("flame.appearance", JSON.stringify({ state: { theme } }));
  const bootstrap = html.match(/<script>([\s\S]*?)<\/script>/)?.[1];
  expect(bootstrap).toBeDefined();
  Function(bootstrap!)();
  expect(document.documentElement.classList.contains("theme-light")).toBe(true);
  expect(document.documentElement.style.backgroundColor).toBe("#fafafa");
  expect(resolveThemeScheme(owner.access.system, theme)).toBe("dark");
});

it.each([
  ["light", "#f7f4f3", "#f5f5f5"],
  ["dark", "#110e0d", "#0f0f0f"],
] as const)(
  "opens built-in %s on its neutral canvas despite an old tinted paint",
  (theme, old, canvas) => {
    rememberThemePaint(() => localStorage, theme, theme, old);
    localStorage.setItem("flame.appearance", JSON.stringify({ state: { theme } }));

    Function(html.match(/<script>([\s\S]*?)<\/script>/)![1]!)();

    expect(document.documentElement.className).toBe(`theme-${theme}`);
    expect(document.documentElement.style.backgroundColor).toBe(canvas);
  },
);

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
  rememberThemePaint(() => localStorage, theme, "light", "#fafafa");
  expect(owner.store.getState().theme).not.toBe(theme);
  owner.store.setState({ theme });
  retainThemeSelection(owner.access.preference, [theme], "package:");
  expect(owner.store.getState().theme).toBe(theme);
  retainThemeSelection(owner.access.preference, [], "package:");
  expect(owner.store.getState().theme).toBe("system");
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
  owner.store.setState({ theme: "light" });
  retainThemeSelection(owner.access.preference, [], "package:");
  expect(owner.store.getState().theme).toBe("light");
});

it("forgets the paint of a package theme Runtime stops admitting so cold boot follows the system", async () => {
  const uninstall = owner.start();
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
    owner.store.setState({ theme });
    expect(localStorage.getItem("flame.theme-paint")).toContain(`"theme":"${theme}"`);

    retainThemeSelection(owner.access.preference, [], "package:");

    expect(owner.store.getState().theme).toBe("system");
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
