import { colord } from "colord";
import type { StoreApi } from "zustand";
import type { AppearancePreference, ColorThemeId, VisualStyleId } from "../kit/appearance";
import {
  publishMotionScale,
  publishScheme,
  publishTokens,
  publishVisualStyleMotion,
} from "@/lib/appearance";
import { densityCssVariables } from "../kit/density";
import { iconScaleCssVariables } from "@/lib/iconScale";
import { uiTypeLadderCssVariables } from "../kit/typeLadder";
import { ACCENT, COLOR_THEME, VISUAL_STYLE } from "@/plugins/sdk/kernelPoints";
import { subscribeContributions } from "@/plugins/sdk";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import {
  WCAG_AA_NON_TEXT,
  WCAG_AA_TEXT,
  edgeOnCanvas,
  focusOnCanvas,
  inkOnFill,
} from "../kit/legibility";
import { depthStep } from "../kit/tokens";
import { visualStyleMotionTokens } from "../visualStyles/tokens";
import { resolveThemeScheme } from "../application/themeScheme";
import { subscribeSystemScheme } from "./systemAppearance";

type UiEffectStore<T extends AppearancePreference> = Pick<StoreApi<T>, "getState" | "subscribe">;

function lightAccent(darkHex: string): string {
  const preset = lookupExtensionPoint(ACCENT).find((accent) => accent.dark === darkHex);
  return preset?.light ?? preset?.dark ?? colord(darkHex).darken(0.2).toHex();
}

function replaceTokens(previous: string[], tokens: Record<string, string>): string[] {
  const root = document.documentElement;
  for (const property of previous) root.style.removeProperty(property);
  const next: string[] = [];
  for (const [name, value] of Object.entries(tokens)) {
    const property = `--${name}`;
    root.style.setProperty(property, value);
    next.push(property);
  }
  return next;
}

let appliedColorTokens: string[] = [];
let appliedStyleTokens: string[] = [];

function applyColorTheme(theme: ColorThemeId, accent: string, contrast: number): void {
  const root = document.documentElement;
  const scheme = resolveThemeScheme(theme);
  const spec = lookupExtensionByKey(COLOR_THEME, theme === "system" ? scheme : theme);

  root.classList.remove("theme-light", "theme-dark");
  root.classList.add(`theme-${scheme}`);

  const liveAccent = scheme === "light" ? lightAccent(accent) : accent;
  appliedColorTokens = replaceTokens(appliedColorTokens, {
    ...spec?.tokens,
  });

  root.style.setProperty("--color-accent", liveAccent);
  root.style.setProperty("--color-accent-border", colord(liveAccent).darken(0.08).toHex());
  root.style.setProperty("--color-accent-press", colord(liveAccent).darken(0.16).toHex());
  root.style.setProperty("--depth-step", depthStep(scheme, contrast));

  const declaredInk = spec?.tokens?.["color-text-on-accent"] ?? "#ffffff";
  const resolved = (name: string) => getComputedStyle(root).getPropertyValue(name).trim();
  root.style.setProperty(
    "--color-text-on-accent",
    inkOnFill(declaredInk, resolved("--color-accent"), WCAG_AA_NON_TEXT),
  );
  root.style.setProperty(
    "--color-cta-text",
    inkOnFill(declaredInk, resolved("--color-cta"), WCAG_AA_TEXT),
  );
  root.style.setProperty(
    "--color-focus-ring",
    focusOnCanvas(resolved("--color-accent"), resolved("--color-text"), resolved("--color-bg")),
  );
  root.style.setProperty(
    "--color-control-edge",
    edgeOnCanvas(resolved("--color-text"), resolved("--color-bg")),
  );
  appliedColorTokens.push(
    "--color-accent",
    "--color-accent-border",
    "--color-accent-press",
    "--color-text-on-accent",
    "--color-cta-text",
    "--color-focus-ring",
    "--color-control-edge",
    "--depth-step",
  );

  publishScheme(scheme);
}

function applyVisualStyle(id: VisualStyleId): void {
  const root = document.documentElement;
  const spec =
    lookupExtensionByKey(VISUAL_STYLE, id) ?? lookupExtensionByKey(VISUAL_STYLE, "flame");
  const motionTokens = spec ? visualStyleMotionTokens(spec.motion) : {};
  appliedStyleTokens = replaceTokens(appliedStyleTokens, { ...spec?.tokens, ...motionTokens });
  if (spec) publishVisualStyleMotion(spec.motion);
  root.dataset.visualStyle = spec?.id ?? "flame";
}

function applyFonts(
  uiFont: string,
  codeFont: string,
  fontSize: number | null,
  fontSmoothing: boolean,
): void {
  const root = document.documentElement;
  root.style.setProperty("-webkit-font-smoothing", fontSmoothing ? "antialiased" : "auto");

  if (uiFont) {
    root.style.setProperty(
      "--font-sans",
      `"${uiFont}", -apple-system, system-ui, "PingFang SC", sans-serif`,
    );
  } else {
    root.style.removeProperty("--font-sans");
  }

  if (codeFont) {
    root.style.setProperty(
      "--font-mono",
      `"${codeFont}", ui-monospace, "SF Mono", Menlo, monospace`,
    );
  } else {
    root.style.removeProperty("--font-mono");
  }

  for (const [property, value] of Object.entries({
    ...uiTypeLadderCssVariables(fontSize),
    ...iconScaleCssVariables(fontSize),
  })) {
    root.style.setProperty(property, value);
  }
}

function applyShape(density: string, radiusScale: number, motionScale: number): void {
  const root = document.documentElement;
  for (const [property, value] of Object.entries(densityCssVariables(density))) {
    root.style.setProperty(property, value);
  }
  root.style.setProperty("--radius-scale", String(radiusScale));
  root.style.setProperty("--motion-scale", String(motionScale));
  publishMotionScale(motionScale);
  if (motionScale === 0) root.setAttribute("data-motion", "off");
  else root.removeAttribute("data-motion");
}

export function installDocumentAppearance<T extends AppearancePreference>(
  store: UiEffectStore<T>,
): () => void {
  const initial = store.getState();
  applyColorTheme(initial.theme, initial.accent, initial.contrast);
  applyVisualStyle(initial.visualStyle);
  publishTokens();
  applyFonts(initial.uiFont, initial.codeFont, initial.fontSize, initial.fontSmoothing);
  applyShape(initial.density, initial.radiusScale, initial.motionScale);

  const unsubscribeUi = store.subscribe((state, previous) => {
    if (
      state.theme !== previous.theme ||
      state.accent !== previous.accent ||
      state.contrast !== previous.contrast
    ) {
      applyColorTheme(state.theme, state.accent, state.contrast);
      applyVisualStyle(state.visualStyle);
      publishTokens();
    } else if (state.visualStyle !== previous.visualStyle) {
      applyVisualStyle(state.visualStyle);
      publishTokens();
    }
    if (
      state.uiFont !== previous.uiFont ||
      state.codeFont !== previous.codeFont ||
      state.fontSize !== previous.fontSize ||
      state.fontSmoothing !== previous.fontSmoothing
    ) {
      applyFonts(state.uiFont, state.codeFont, state.fontSize, state.fontSmoothing);
    }
    if (
      state.density !== previous.density ||
      state.radiusScale !== previous.radiusScale ||
      state.motionScale !== previous.motionScale
    ) {
      applyShape(state.density, state.radiusScale, state.motionScale);
    }
  });

  const unsubscribePlugins = subscribeContributions(() => {
    const current = store.getState();
    applyColorTheme(current.theme, current.accent, current.contrast);
    applyVisualStyle(current.visualStyle);
    publishTokens();
  });

  const unsubscribeScheme = subscribeSystemScheme(() => {
    const current = store.getState();
    if (current.theme !== "system") return;
    applyColorTheme(current.theme, current.accent, current.contrast);
    applyVisualStyle(current.visualStyle);
    publishTokens();
  });

  return () => {
    unsubscribeScheme();
    unsubscribePlugins();
    unsubscribeUi();
  };
}
