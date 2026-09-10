import { colord } from "colord";
import type { StoreApi } from "zustand";
import type {
  AccentTint,
  AppearancePreference,
  ColorThemeId,
  VisualStyleId,
} from "../kit/appearance";
import {
  publishMotionScale,
  publishScheme,
  publishTokens,
  publishVisualStyleMotion,
} from "@/lib/appearance";
import { densityCssVariables } from "../kit/density";
import { iconScaleCssVariables } from "@/lib/iconScale";
import { uiTypeLadderCssVariables } from "../kit/typeLadder";
import type { ColorThemeSpec, NeutralStep } from "@/plugins/sdk";
import { ACCENT, COLOR_THEME, VISUAL_STYLE } from "@/plugins/sdk/kernelPoints";
import { subscribeContributions } from "@/plugins/sdk";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { accentTintedNeutral } from "../kit/accentTint";
import { WCAG_AA_NON_TEXT, WCAG_AA_TEXT, inkOnFill } from "../kit/legibility";
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

/**
 * A theme's `surfaces` / `borders` literals are already that family at the DEFAULT accent —
 * what the pre-paint script and stylesheet mirror carry — so this returns an OVERRIDE, and
 * nothing at all for a palette theme: its own surface is its own, not a tint.
 */
function neutralOverride(
  spec: ColorThemeSpec | undefined,
  liveAccent: string,
  tint: AccentTint,
): Record<string, string> {
  const steps = spec?.neutralSteps;
  // The theme's own accent is the reference the derivation is relative to, so an
  // untouched accent reproduces its literals byte for byte. A theme whose accent is not
  // a plain hex (a palette pointing at a var) opts out by having nothing to measure
  // against.
  const reference = spec?.tokens?.["color-accent"];
  if (!steps || !reference || !/^#[\da-f]{6}$/i.test(reference)) return {};
  const tinted = (step: NeutralStep) => accentTintedNeutral(liveAccent, reference, step, tint);
  return {
    "color-surface": tinted(steps.surface),
    "color-elevated": tinted(steps.elevated),
    "color-sunken": tinted(steps.sunken),
    "color-border": tinted(steps.border),
    "color-border-soft": tinted(steps.borderSoft),
  };
}

/**
 * The ink that sits ON the accent, which has to follow the accent.
 *
 * A theme declares one — white, for the blue it ships with — and the accent is a colour the
 * user picks freely, so the two came apart the moment anyone chose a pale one: white on a soft
 * yellow measured 1.39:1 against the 4.5:1 the criterion asks for, and `--color-cta-text` reads
 * this token, so every primary button went with it.
 *
 * The theme's own choice is kept whenever it reads, so a palette that has thought about this
 * keeps its answer and the default is untouched; only an accent that breaks it gets overruled,
 * by whichever pole is further from it.
 */
function applyColorTheme(
  theme: ColorThemeId,
  accent: string,
  contrast: number,
  accentTint: AccentTint,
): void {
  const root = document.documentElement;
  const scheme = resolveThemeScheme(theme);
  const spec = lookupExtensionByKey(COLOR_THEME, theme === "system" ? scheme : theme);

  root.classList.remove("theme-light", "theme-dark");
  root.classList.add(`theme-${scheme}`);

  // The accent's hover and press shades follow the LIVE accent, not the theme's
  // declared one, keeping every interaction state on the selected hue.
  const liveAccent = scheme === "light" ? lightAccent(accent) : accent;
  appliedColorTokens = replaceTokens(appliedColorTokens, {
    ...spec?.tokens,
    ...neutralOverride(spec, liveAccent, accentTint),
  });

  root.style.setProperty("--color-accent", liveAccent);
  root.style.setProperty("--color-accent-border", colord(liveAccent).darken(0.08).toHex());
  root.style.setProperty("--color-accent-press", colord(liveAccent).darken(0.16).toHex());
  root.style.setProperty("--depth-step", depthStep(scheme, contrast));

  // Ink follows the fill it will sit ON, and there are two of them. A mark sits on
  // `--color-accent`; a button's label sits on `--color-cta`, which a theme may define as a
  // different shade — flame's dark CTA is the accent's border shade, two steps of luminance
  // away. One token served both, so it could not be right for both, and the accent is a colour
  // the user picks freely: white on a soft yellow measured 1.39:1.
  //
  // Read back what the browser resolved rather than recomputing each theme's expression here,
  // so a palette that defines its CTA some other way is covered by the same two lines.
  //
  // This works because of exactly one property of custom properties, and it is worth naming so
  // nobody "fixes" it: `getPropertyValue` hands back the COMPUTED value, and computing a custom
  // property substitutes `var()`. `globals.css` says `--color-cta: var(--color-accent)` and this
  // reads `#2b5fd0` — measured, along with a pale accent arriving as `#ffcb00` and the ink below
  // correctly flipping to black.
  //
  // What computing a custom property does NOT do is evaluate anything else. A token authored as
  // `color-mix(…)` or `calc(…)` arrives here as that text — `--color-text-muted` and
  // `--color-surface-2` both would — and `colord` cannot parse it. So this pair is safe only
  // while the CTA and the accent resolve to a colour literal or a chain of `var()`s to one; a
  // palette that mixes its CTA needs the value painted onto a probe and read back instead.
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
  appliedColorTokens.push(
    "--color-accent",
    "--color-accent-border",
    "--color-accent-press",
    "--color-text-on-accent",
    "--color-cta-text",
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

  // The icon ladder rides the same base: a glyph beside a label must grow with it,
  // and its stroke is derived from the size it lands on.
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
  applyColorTheme(initial.theme, initial.accent, initial.contrast, initial.accentTint);
  applyVisualStyle(initial.visualStyle);
  publishTokens();
  applyFonts(initial.uiFont, initial.codeFont, initial.fontSize, initial.fontSmoothing);
  applyShape(initial.density, initial.radiusScale, initial.motionScale);

  const unsubscribeUi = store.subscribe((state, previous) => {
    if (
      state.theme !== previous.theme ||
      state.accent !== previous.accent ||
      state.contrast !== previous.contrast ||
      state.accentTint !== previous.accentTint
    ) {
      applyColorTheme(state.theme, state.accent, state.contrast, state.accentTint);
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
    applyColorTheme(current.theme, current.accent, current.contrast, current.accentTint);
    applyVisualStyle(current.visualStyle);
    publishTokens();
  });

  const unsubscribeScheme = subscribeSystemScheme(() => {
    const current = store.getState();
    if (current.theme !== "system") return;
    applyColorTheme(current.theme, current.accent, current.contrast, current.accentTint);
    applyVisualStyle(current.visualStyle);
    publishTokens();
  });

  return () => {
    unsubscribeScheme();
    unsubscribePlugins();
    unsubscribeUi();
  };
}
