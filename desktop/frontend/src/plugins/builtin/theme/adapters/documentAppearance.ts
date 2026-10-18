import type { StoreApi } from "zustand";
import type { AppearancePreference, ColorThemeId, VisualStyleId } from "../kit/appearance";
import {
  publishMotionScale,
  publishScheme,
  publishTokens,
  publishVisualStyleMotion,
  publishUiFontSize,
} from "@/lib/appearance";
import { densityCssVariables } from "../kit/density";
import { productIconScaleCssVariables } from "@/lib/productIconScale";
import { uiTypeLadderCssVariables } from "../kit/typeLadder";
import { COLOR_THEME, VISUAL_STYLE } from "@/plugins/sdk/kernelPoints";
import { subscribeContributions } from "@/plugins/sdk";
import { lookupExtensionByKey } from "@/plugins/sdk/selectors/extensions";
import { depthStep } from "../kit/tokens";
import { visualStyleMotionTokens } from "../visualStyles/tokens";
import { resolveThemeScheme } from "../application/themeScheme";
import { subscribeSystemScheme } from "./systemAppearance";
import { forgetThemePaint, rememberThemePaint } from "./themePaint";

type UiEffectStore<T extends AppearancePreference> = Pick<StoreApi<T>, "getState" | "subscribe">;

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

function applyColorTheme(theme: ColorThemeId, contrast: number): void {
  const root = document.documentElement;
  const scheme = resolveThemeScheme(theme);
  const spec = lookupExtensionByKey(COLOR_THEME, theme === "system" ? scheme : theme);

  root.classList.remove("theme-light", "theme-dark");
  root.classList.add(`theme-${scheme}`);

  appliedColorTokens = replaceTokens(appliedColorTokens, {
    ...spec?.tokens,
    "depth-step": depthStep(scheme, contrast),
  });

  publishScheme(scheme);
  const background = getComputedStyle(root).getPropertyValue("--color-bg").trim();
  root.style.backgroundColor = background;
  if (spec && theme !== "system") rememberThemePaint(theme, scheme, background);
  else forgetThemePaint();
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
  codeFontSize: number | null,
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
    ...productIconScaleCssVariables(fontSize),
    ...(codeFontSize === null ? {} : { "--fs-code": `${codeFontSize}px` }),
  })) {
    root.style.setProperty(property, value);
  }
  publishUiFontSize(fontSize);
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
  applyColorTheme(initial.theme, initial.contrast);
  applyVisualStyle(initial.visualStyle);
  publishTokens();
  applyFonts(
    initial.uiFont,
    initial.codeFont,
    initial.fontSize,
    initial.codeFontSize,
    initial.fontSmoothing,
  );
  applyShape(initial.density, initial.radiusScale, initial.motionScale);

  const unsubscribeUi = store.subscribe((state, previous) => {
    if (
      state.theme !== previous.theme ||
      state.accent !== previous.accent ||
      state.contrast !== previous.contrast
    ) {
      applyColorTheme(state.theme, state.contrast);
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
      state.codeFontSize !== previous.codeFontSize ||
      state.fontSmoothing !== previous.fontSmoothing
    ) {
      applyFonts(
        state.uiFont,
        state.codeFont,
        state.fontSize,
        state.codeFontSize,
        state.fontSmoothing,
      );
    }
    if (
      state.density !== previous.density ||
      state.radiusScale !== previous.radiusScale ||
      state.motionScale !== previous.motionScale
    ) {
      applyShape(state.density, state.radiusScale, state.motionScale);
    }
    publishTokens();
  });

  const unsubscribePlugins = subscribeContributions(() => {
    const current = store.getState();
    applyColorTheme(current.theme, current.contrast);
    applyVisualStyle(current.visualStyle);
    publishTokens();
  });

  const unsubscribeScheme = subscribeSystemScheme(() => {
    const current = store.getState();
    if (current.theme !== "system") return;
    applyColorTheme(current.theme, current.contrast);
    applyVisualStyle(current.visualStyle);
    publishTokens();
  });

  return () => {
    unsubscribeScheme();
    unsubscribePlugins();
    unsubscribeUi();
  };
}
