import { THEME_CLASS_PREFIX } from "@/features/theme/kit/documentTheme";
import { type StoreApi } from "zustand";
import {
  type AppearancePreference,
  type ColorThemeId,
  type VisualStyleId,
} from "@/features/theme/kit/appearance";
import { type AppearancePublisher } from "@/lib/appearance";
import { densityCssVariables } from "@/features/theme/kit/density";
import { productIconScaleCssVariables } from "@/lib/productIconScale";
import { uiTypeLadderCssVariables } from "@/features/theme/kit/typeLadder";
import { paperTextureImage } from "@/features/theme/kit/paperTexture";
import { COLOR_THEME, VISUAL_STYLE } from "@/features/theme/public/extensionPoints";
import { subscribeContributions } from "@/extensions/kernel";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { depthStep } from "@/features/theme/kit/tokens";
import { visualStyleMotionTokens } from "@/features/theme/visualStyles/tokens";
import { resolveThemeScheme } from "@/features/theme/application/themeScheme";
import { type SystemAppearance } from "@/features/theme/application/systemAppearance";
import { forgetThemePaint, rememberThemePaint } from "@/features/theme/adapters/themePaint";
type UiEffectStore<T extends AppearancePreference> = Pick<StoreApi<T>, "getState" | "subscribe">;

const documentPainters = new WeakMap<Document, () => void>();

export function installDocumentAppearance<T extends AppearancePreference>(
  document: Document,
  window: Window,
  store: UiEffectStore<T>,
  system: SystemAppearance,
  appearance: AppearancePublisher,
): () => void {
  documentPainters.get(document)?.();
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
    const scheme = resolveThemeScheme(system, theme);
    const spec = lookupExtensionByKey(COLOR_THEME, theme === "system" ? scheme : theme);

    root.classList.remove(`${THEME_CLASS_PREFIX}light`, `${THEME_CLASS_PREFIX}dark`);
    root.classList.add(`${THEME_CLASS_PREFIX}${scheme}`);

    appliedColorTokens = replaceTokens(appliedColorTokens, {
      ...spec?.tokens,
      "depth-step": depthStep(scheme, contrast),
    });

    appearance.publishScheme(scheme);
    const background = window.getComputedStyle(root).getPropertyValue("--color-bg").trim();
    root.style.backgroundColor = background;
    if (spec && theme !== "system")
      rememberThemePaint(() => window.localStorage, theme, scheme, background);
    else forgetThemePaint(() => window.localStorage);
  }

  function applyVisualStyle(id: VisualStyleId): void {
    const root = document.documentElement;
    const spec =
      lookupExtensionByKey(VISUAL_STYLE, id) ?? lookupExtensionByKey(VISUAL_STYLE, "flame");
    const motionTokens = spec ? visualStyleMotionTokens(spec.motion) : {};
    appliedStyleTokens = replaceTokens(appliedStyleTokens, { ...spec?.tokens, ...motionTokens });
    if (spec) appearance.publishVisualStyleMotion(spec.motion);
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
    appearance.publishUiFontSize(fontSize);
  }

  function applyShape(density: string, radiusScale: number, motionScale: number): void {
    const root = document.documentElement;
    for (const [property, value] of Object.entries(densityCssVariables(density))) {
      root.style.setProperty(property, value);
    }
    root.style.setProperty("--radius-scale", String(radiusScale));
    root.style.setProperty("--motion-scale", String(motionScale));
    appearance.publishMotionScale(motionScale);
    if (motionScale === 0) root.setAttribute("data-motion", "off");
    else root.removeAttribute("data-motion");
  }

  function applyPaperTexture(preference: AppearancePreference): void {
    const root = document.documentElement;
    if (preference.paperTexture === "off") root.removeAttribute("data-paper-texture");
    else root.dataset.paperTexture = preference.paperTexture;
    root.style.setProperty(
      "--surface-texture",
      paperTextureImage(
        preference.paperTexture,
        preference.paperTextureStrength,
        resolveThemeScheme(system, preference.theme),
      ),
    );
  }

  const initial = store.getState();
  applyColorTheme(initial.theme, initial.contrast);
  applyVisualStyle(initial.visualStyle);
  appearance.publishTokens();
  applyFonts(
    initial.uiFont,
    initial.codeFont,
    initial.fontSize,
    initial.codeFontSize,
    initial.fontSmoothing,
  );
  applyShape(initial.density, initial.radiusScale, initial.motionScale);
  applyPaperTexture(initial);

  const unsubscribeUi = store.subscribe((state, previous) => {
    if (
      state.theme !== previous.theme ||
      state.accent !== previous.accent ||
      state.contrast !== previous.contrast
    ) {
      applyColorTheme(state.theme, state.contrast);
      applyVisualStyle(state.visualStyle);
      appearance.publishTokens();
    } else if (state.visualStyle !== previous.visualStyle) {
      applyVisualStyle(state.visualStyle);
      appearance.publishTokens();
    }
    if (
      state.paperTexture !== previous.paperTexture ||
      state.paperTextureStrength !== previous.paperTextureStrength ||
      state.theme !== previous.theme
    ) {
      applyPaperTexture(state);
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
    appearance.publishTokens();
  });

  const unsubscribePlugins = subscribeContributions(() => {
    const current = store.getState();
    applyColorTheme(current.theme, current.contrast);
    applyVisualStyle(current.visualStyle);
    applyPaperTexture(current);
    appearance.publishTokens();
  });

  const unsubscribeScheme = system.subscribe(() => {
    const current = store.getState();
    if (current.theme !== "system") return;
    applyColorTheme(current.theme, current.contrast);
    applyVisualStyle(current.visualStyle);
    applyPaperTexture(current);
    appearance.publishTokens();
  });

  let disposed = false;
  const dispose = () => {
    if (disposed) return;
    disposed = true;
    unsubscribeScheme();
    unsubscribePlugins();
    unsubscribeUi();
    if (documentPainters.get(document) !== dispose) return;
    documentPainters.delete(document);
    replaceTokens(appliedColorTokens, {});
    replaceTokens(appliedStyleTokens, {});
  };
  documentPainters.set(document, dispose);
  return dispose;
}
