import { beforeEach, describe, expect, it } from "vitest";
import { act, render } from "@testing-library/react";
import { createElement } from "react";
import { Icon } from "@/ui/icons/index";
import { COLOR_THEME, VISUAL_STYLE } from "@/features/theme/public/extensionPoints";
import { useAppearanceStore } from "@/features/theme/adapters/appearanceStore";
import { installDocumentAppearance } from "@/features/theme/adapters/documentAppearance";
import { installSystemAppearance } from "@/features/theme/adapters/systemAppearance";
import { toggleThemeScheme } from "@/features/theme/application/themeScheme";
import { installAppearancePreferencePort } from "@/features/theme/adapters/appearancePreferenceBinding";
import { type PluginContext } from "@/extensions/definePlugin";
import { contributeForTest, loadPluginsForTest } from "@/test/kernel";
import flameDark from "@/features/theme/themes/flame-dark";
import flameLight from "@/features/theme/themes/flame-light";
import customTheme from "@/features/theme/themes/custom-theme";
import { contrastRatio, WCAG_AA_NON_TEXT } from "@/features/theme/kit/legibility";
let uninstall: () => void = () => {};

const TEST_MOTION = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  disclosureMs: 220,
  slowMs: 360,
  drawerMs: 500,
  easeOut: [0.22, 1, 0.36, 1],
  easeInOut: [0.45, 0, 0.55, 1],
  easeEmphasized: [0.16, 1, 0.3, 1],
  drawerProgress: [0, 0.5, 1],
  pressScale: 0.96,
} as const;

beforeEach(() => {
  document.documentElement.removeAttribute("style");
  document.documentElement.className = "";
  useAppearanceStore.setState({
    theme: "dark",
    visualStyle: "synara",
    accent: "#1ed760",
    contrast: 60,
    uiFont: "",
    codeFont: "",
    fontSize: null,
    codeFontSize: null,
    fontSmoothing: true,
    radiusScale: 1,
    motionScale: 1,
    paperTexture: "off",
    paperTextureStrength: 3,
  });
  uninstall();
  installAppearancePreferencePort();
  installSystemAppearance();
  uninstall = installDocumentAppearance(useAppearanceStore);
});

async function registerSchemePair(): Promise<void> {
  await contributeForTest((ctx) => {
    ctx.contribute(COLOR_THEME, { id: "dark", label: "Dark", scheme: "dark" });
    ctx.contribute(COLOR_THEME, { id: "light", label: "Light", scheme: "light" });
  }, "test.schemes");
}

describe("paper texture on the reading planes", () => {
  const texture = () => document.documentElement.style.getPropertyValue("--surface-texture");

  it("starts plain and keeps the chosen strength when paper is turned off", () => {
    expect(texture()).toBe("none");
    expect(document.documentElement.hasAttribute("data-paper-texture")).toBe(false);
    useAppearanceStore.getState().setPaperTexture("xuan");
    expect(document.documentElement.dataset.paperTexture).toBe("xuan");
    const first = texture();
    expect(first).toContain("data:image/svg+xml,");
    useAppearanceStore.getState().setPaperTextureStrength(5);
    expect(texture()).not.toBe(first);
    useAppearanceStore.getState().setPaperTexture("off");
    expect(texture()).toBe("none");
    expect(document.documentElement.hasAttribute("data-paper-texture")).toBe(false);
    expect(useAppearanceStore.getState().paperTextureStrength).toBe(5);
  });

  it("changes paper and ink with the theme without tinting its neutral planes", async () => {
    await loadPluginsForTest(flameLight, flameDark);
    useAppearanceStore.setState({ theme: "light" });
    const planes = ["color-bg", "color-drawer", "color-surface", "color-elevated"];
    const readPlanes = () =>
      planes.map((name) => document.documentElement.style.getPropertyValue(`--${name}`));
    const plain = readPlanes();
    const images: string[] = [];
    for (const paperTexture of ["xuan", "cotton", "grain"] as const) {
      useAppearanceStore.setState({ paperTexture });
      images.push(texture());
      expect(readPlanes()).toEqual(plain);
    }
    expect(new Set(images).size).toBe(3);
    useAppearanceStore.getState().setTheme("dark");
    expect(texture()).not.toBe(images[2]);
    useAppearanceStore.getState().setTheme("light");
    expect(texture()).toBe(images[2]);
    expect(readPlanes()).toEqual(plain);
  });

  it("updates paper ink when editing a custom theme changes its scheme", async () => {
    await loadPluginsForTest(customTheme);
    useAppearanceStore.setState({
      theme: "custom",
      customTheme: { bg: "#eee7db", fg: "#232323" },
      paperTexture: "xuan",
    });
    const light = texture();
    expect(document.documentElement.classList.contains("theme-light")).toBe(true);
    useAppearanceStore.getState().setCustomTheme({ bg: "#161616", fg: "#eeeeee" });
    expect(document.documentElement.classList.contains("theme-dark")).toBe(true);
    expect(texture()).not.toBe(light);
    useAppearanceStore.getState().setCustomTheme({ bg: "#eee7db", fg: "#232323" });
    expect(texture()).toBe(light);
  });

  it("repaints built-in reading ink as paper strength changes and restores it on Off", async () => {
    await loadPluginsForTest(flameLight, flameDark);
    useAppearanceStore.setState({ theme: "light", contrast: 25 });
    const faint = () => document.documentElement.style.getPropertyValue("--color-text-faint");
    const plain = faint();
    useAppearanceStore.getState().setPaperTexture("xuan");
    const medium = faint();
    expect(medium).not.toBe(plain);
    useAppearanceStore.getState().setPaperTextureStrength(5);
    expect(faint()).not.toBe(medium);
    useAppearanceStore.getState().setPaperTexture("off");
    expect(faint()).toBe(plain);
  });
});

describe("a theme's own neutrals are what paints", () => {
  const painted = (name: string) => document.documentElement.style.getPropertyValue(`--${name}`);

  function tinted(ctx: PluginContext) {
    ctx.contribute(COLOR_THEME, {
      id: "tinted",
      label: "Tinted",
      scheme: "dark",
      tokens: {
        "color-accent": "#3574f0",
        "color-surface": "#2a2d32",
        "color-sunken": "#14181f",
      },
    });
  }

  it.each([
    ["dark", "#101828"],
    ["light", "#ffe066"],
  ] as const)(
    "publishes readable %s controls through contrast and accent changes",
    async (theme, accent) => {
      await loadPluginsForTest(flameLight, flameDark);
      useAppearanceStore.setState({ theme, accent });
      const reads = () => {
        for (const name of ["color-control-edge", "color-focus-ring"])
          for (const plane of [
            "color-bg",
            "color-drawer",
            "color-surface",
            "color-elevated",
            "color-sunken",
          ])
            expect(contrastRatio(painted(name), painted(plane))).toBeGreaterThanOrEqual(
              WCAG_AA_NON_TEXT,
            );
      };
      for (const contrast of [0, 25, 100]) {
        useAppearanceStore.getState().setContrast(contrast);
        reads();
      }
      useAppearanceStore.getState().setAccent("#7f52ff");
      reads();
    },
  );

  it("reproduces the theme's own literals when the accent is untouched", async () => {
    await contributeForTest((ctx) => {
      tinted(ctx);
    });
    useAppearanceStore.setState({ theme: "tinted", accent: "#3574f0" });

    expect(painted("color-surface")).toBe("#2a2d32");
    expect(painted("color-sunken")).toBe("#14181f");
  });

  it("holds them still through any accent", async () => {
    await contributeForTest((ctx) => {
      tinted(ctx);
    });
    useAppearanceStore.setState({ theme: "tinted", accent: "#3574f0" });
    const declared = painted("color-sunken");

    for (const accent of ["#e8590c", "#000000", "#7f52ff"]) {
      useAppearanceStore.setState({ accent });
      expect(
        painted("color-sunken"),
        "a surface is the theme's to state, not the accent's to move",
      ).toBe(declared);
    }
  });

  it("does not touch a palette theme that never opted in", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "palette",
        label: "Palette",
        scheme: "light",
        tokens: { "color-accent": "#268bd2", "color-surface": "#eee8d5" },
      });
    });
    useAppearanceStore.setState({ theme: "palette", accent: "#7f52ff" });

    expect(painted("color-surface")).toBe("#eee8d5");
  });

  it.each([
    ["light", "#f5f5f5"],
    ["dark", "#0f0f0f"],
  ] as const)(
    "keeps the built-in %s planes still while the accent moves",
    async (theme, canvas) => {
      await loadPluginsForTest(flameLight, flameDark);
      useAppearanceStore.setState({ theme, accent: "#fb6e30" });
      const planes = [
        "color-bg",
        "color-drawer",
        "color-surface",
        "color-elevated",
        "color-sunken",
        "color-text",
        "color-divider",
        "color-border",
        "color-control-edge",
      ];
      const paint = () => planes.map(painted);
      const declared = paint();
      expect(painted("color-bg")).toBe(canvas);

      let accent = painted("color-accent");
      for (const next of ["#0485f7", "#7f52ff", "#21a179"]) {
        useAppearanceStore.getState().setAccent(next);
        expect(painted("color-accent")).not.toBe(accent);
        expect(paint(), "the accent tints its own roles, never the scheme's planes").toEqual(
          declared,
        );
        accent = painted("color-accent");
      }
    },
  );
});

describe("accent ownership", () => {
  const painted = () => document.documentElement.style.getPropertyValue("--color-accent");

  it("paints the accent its theme derived, never a second one of its own", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "declared",
        label: "Declared",
        scheme: "light",
        tokens: { "color-accent": "#b4004e" },
      });
    });

    useAppearanceStore.setState({ theme: "declared", accent: "#1ed760" });
    expect(painted()).toBe("#b4004e");
    useAppearanceStore.setState({ accent: "#7f52ff" });
    expect(painted()).toBe("#b4004e");
  });
});

describe("applyTheme — theme-as-plugin contract", () => {
  it("writes spec.tokens to :root.style when the active theme is registered", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "dark",
        label: "Dark",
        scheme: "dark",
        tokens: {
          "color-bg": "#101010",
          "color-surface": "#1a1a1a",
        },
      });
    });

    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#101010");
    expect(root.style.getPropertyValue("--color-surface")).toBe("#1a1a1a");
  });

  it("toggles theme-{scheme} class — drives structural CSS overrides", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "palette-light",
        label: "Palette Light",
        scheme: "light",
        tokens: { "color-bg": "#fdf6e3" },
      });
    });

    useAppearanceStore.getState().setTheme("palette-light");

    const root = document.documentElement;
    expect(root.classList.contains("theme-light")).toBe(true);
    expect(root.classList.contains("theme-dark")).toBe(false);
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fdf6e3");
  });

  it("switching themes replaces token values", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "dark",
        label: "Dark",
        scheme: "dark",
        tokens: { "color-bg": "#010102", "color-text": "#f7f8f8" },
      });
      ctx.contribute(COLOR_THEME, {
        id: "light",
        label: "Light",
        scheme: "light",
        tokens: { "color-bg": "#fafafa", "color-text": "#171717" },
      });
    });

    useAppearanceStore.getState().setTheme("light");

    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fafafa");
    expect(root.style.getPropertyValue("--color-text")).toBe("#171717");

    useAppearanceStore.getState().setTheme("dark");
    expect(root.style.getPropertyValue("--color-bg")).toBe("#010102");
    expect(root.style.getPropertyValue("--color-text")).toBe("#f7f8f8");
  });

  it("removes tokens omitted by the next theme", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "dark",
        label: "Dark",
        scheme: "dark",
        tokens: { "color-bg": "#010102", "color-warning": "#f0c000" },
      });
      ctx.contribute(COLOR_THEME, {
        id: "light",
        label: "Light",
        scheme: "light",
        tokens: { "color-bg": "#fafafa" },
      });
    });

    useAppearanceStore.getState().setTheme("light");

    const root = document.documentElement;
    expect(root.style.getPropertyValue("--color-bg")).toBe("#fafafa");
    expect(root.style.getPropertyValue("--color-warning")).toBe("");
  });

  it("toggleTheme flips to the first registered theme of the opposite scheme", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(COLOR_THEME, {
        id: "dark",
        label: "Dark",
        scheme: "dark",
        order: 0,
        tokens: {},
      });
      ctx.contribute(COLOR_THEME, {
        id: "palette-light",
        label: "Palette Light",
        scheme: "light",
        order: 0,
        tokens: {},
      });
    });

    useAppearanceStore.setState({ theme: "dark" });
    toggleThemeScheme();
    expect(useAppearanceStore.getState().theme).toBe("palette-light");

    toggleThemeScheme();
    expect(useAppearanceStore.getState().theme).toBe("dark");
  });
});

describe("visual-style contract", () => {
  it("applies component tokens independently from colour", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(VISUAL_STYLE, {
        id: "test-style",
        motion: TEST_MOTION,
        tokens: { "style-shape-md": "5px", "app-content-shadow": "none" },
      });
    });

    useAppearanceStore.getState().setVisualStyle("test-style");

    const root = document.documentElement;
    expect(root.dataset.visualStyle).toBe("test-style");
    expect(root.style.getPropertyValue("--style-shape-md")).toBe("5px");
    expect(root.style.getPropertyValue("--app-content-shadow")).toBe("none");
    expect(root.style.getPropertyValue("--dur-fast-base")).toBe("150ms");
    expect(root.style.getPropertyValue("--ease-out")).toBe("cubic-bezier(0.22, 1, 0.36, 1)");
    expect(root.style.getPropertyValue("--ease-drawer")).toBe("linear(0, 0.5, 1)");
  });

  it("removes tokens omitted by the next visual style", async () => {
    await contributeForTest((ctx) => {
      ctx.contribute(VISUAL_STYLE, {
        id: "first",
        motion: TEST_MOTION,
        tokens: { "shadow-surface-card": "0 8px 20px black" },
      });
      ctx.contribute(VISUAL_STYLE, {
        id: "second",
        motion: TEST_MOTION,
        tokens: {},
      });
    });

    useAppearanceStore.getState().setVisualStyle("first");
    useAppearanceStore.getState().setVisualStyle("second");

    expect(document.documentElement.style.getPropertyValue("--shadow-surface-card")).toBe("");
  });
});

describe("UI preference DOM synchronization", () => {
  it("publishes the same font-derived size to SVGs and CSS layout tokens", () => {
    const { container, unmount } = render(createElement(Icon, { name: "bell", size: "lg" }));
    act(() => useAppearanceStore.getState().setFontSize(16));
    const svg = container.querySelector("svg")!;
    expect(document.documentElement.style.getPropertyValue("--icon-lg")).toBe("22px");
    expect(svg.style.width).toBe("22px");
    expect(svg.getAttribute("viewBox")).toBe("0 0 24 24");
    unmount();
  });

  it("applies and clears font preferences", async () => {
    const state = useAppearanceStore.getState();
    state.setUiFont("Inter");
    state.setCodeFont("JetBrains Mono");
    state.setFontSize(17);
    state.setFontSmoothing(false);

    const style = document.documentElement.style;
    expect(style.getPropertyValue("--font-sans")).toContain('"Inter"');
    expect(style.getPropertyValue("--font-mono")).toContain('"JetBrains Mono"');
    expect(style.fontSize).toBe("");
    expect(style.getPropertyValue("--fs-ui-md")).toBe("17px");
    expect(style.getPropertyValue("--fs-prose")).toBe("19px");
    expect(style.getPropertyValue("--fs-ui-sm")).toBe("16px");

    useAppearanceStore.getState().setUiFont("");
    useAppearanceStore.getState().setCodeFont("");
    useAppearanceStore.getState().setFontSize(null);

    expect(style.getPropertyValue("--font-sans")).toBe("");
    expect(style.getPropertyValue("--font-mono")).toBe("");
    expect(style.getPropertyValue("--fs-ui-md")).toBe("14px");
  });

  it("applies contrast, radius, and reduced-motion preferences", async () => {
    await registerSchemePair();
    useAppearanceStore.getState().setTheme("light");
    useAppearanceStore.getState().setContrast(100);
    useAppearanceStore.getState().setRadiusScale(1.25);
    useAppearanceStore.getState().setMotionScale(0);

    const root = document.documentElement;
    expect(root.style.getPropertyValue("--depth-step")).toBe("10%");
    expect(root.style.getPropertyValue("--radius-scale")).toBe("1.25");
    expect(root.style.getPropertyValue("--motion-scale")).toBe("0");
    expect(root.dataset.motion).toBe("off");

    useAppearanceStore.getState().setMotionScale(0.5);
    expect(root.dataset.motion).toBeUndefined();
  });

  it("doubles the ladder step on dark so both schemes separate equally", async () => {
    await registerSchemePair();
    useAppearanceStore.getState().setContrast(25);
    useAppearanceStore.getState().setTheme("light");
    expect(document.documentElement.style.getPropertyValue("--depth-step")).toBe("4%");
    useAppearanceStore.getState().setTheme("dark");
    expect(document.documentElement.style.getPropertyValue("--depth-step")).toBe("8%");
  });
});

describe("code size", () => {
  it("moves code alone, leaving the interface ladder where the UI size put it", () => {
    const read = (name: string) => document.documentElement.style.getPropertyValue(name);
    useAppearanceStore.setState({ fontSize: 14, codeFontSize: null });
    const interfaceSize = read("--fs-ui-md");
    const followed = read("--fs-code");

    useAppearanceStore.setState({ codeFontSize: 16 });
    expect(read("--fs-code")).toBe("16px");
    expect(read("--fs-ui-md")).toBe(interfaceSize);

    useAppearanceStore.setState({ codeFontSize: null });
    expect(read("--fs-code")).toBe(followed);
  });
});

describe("the first-paint record mirrors what renders", () => {
  it("is written for a registered theme and cleared once that theme stops rendering", async () => {
    let unregister: (() => void) | undefined;
    await contributeForTest((ctx) => {
      const theme = ctx.contribute(COLOR_THEME, {
        id: "package:a:sea",
        label: "Sea",
        scheme: "light",
        tokens: { "color-bg": "#f4f8fb" },
      });
      unregister = () => theme.dispose();
    }, "test.package-theme");
    useAppearanceStore.setState({ theme: "package:a:sea" });
    expect(localStorage.getItem("flame.theme-paint")).toContain("package:a:sea");

    unregister?.();
    useAppearanceStore.setState({ theme: "package:a:sea", accent: "#123456" });
    expect(localStorage.getItem("flame.theme-paint")).toBeNull();
  });
});
