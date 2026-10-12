import { themeTokenNames } from "./generated/themeTokenNames";
import { UI_FONT_SIZE_MIN_PX, UI_FONT_SIZE_MAX_PX } from "./typography";

export interface ThemeProjection {
  readonly scheme: "light" | "dark";
  readonly fontSize: number;
  readonly tokens: Readonly<Record<string, string>>;
}

export function captureTheme(
  root: HTMLElement,
  scheme: ThemeProjection["scheme"],
  fontSize: number,
): ThemeProjection {
  const style = root.ownerDocument.defaultView!.getComputedStyle(root);
  return {
    scheme,
    fontSize,
    tokens: Object.fromEntries(
      themeTokenNames.map((name) => [
        name,
        style.getPropertyValue(name).trim(),
      ]),
    ),
  };
}

export function validateTheme(value: unknown): ThemeProjection {
  if (value === null || typeof value !== "object")
    throw new Error("Invalid theme projection.");
  const theme = value as ThemeProjection;
  if (
    (theme.scheme !== "light" && theme.scheme !== "dark") ||
    !Number.isInteger(theme.fontSize) ||
    theme.fontSize < UI_FONT_SIZE_MIN_PX ||
    theme.fontSize > UI_FONT_SIZE_MAX_PX ||
    !theme.tokens ||
    Object.keys(theme.tokens).length !== themeTokenNames.length ||
    themeTokenNames.some(
      (name) =>
        typeof theme.tokens[name] !== "string" ||
        theme.tokens[name]!.length > 4096,
    )
  )
    throw new Error("Invalid theme projection.");
  return theme;
}

const paints = new WeakMap<HTMLElement, number>();

export function applyTheme(root: HTMLElement, value: unknown): void {
  const theme = validateTheme(value);
  const view = root.ownerDocument.defaultView!;
  const previous = paints.get(root);
  if (previous !== undefined) view.cancelAnimationFrame(previous);
  root.dataset.flameUiThemePaint = "";
  for (const name of themeTokenNames)
    root.style.setProperty(name, theme.tokens[name]!);
  root.style.colorScheme = theme.scheme;
  root.dataset.flameUiFontSize = String(theme.fontSize);
  root.getBoundingClientRect();
  paints.set(
    root,
    view.requestAnimationFrame(() => {
      delete root.dataset.flameUiThemePaint;
      paints.delete(root);
    }),
  );
  view.dispatchEvent(new Event("flame.ui.theme"));
}
