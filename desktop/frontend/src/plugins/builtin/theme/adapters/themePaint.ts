import { colord } from "colord";
import type { Scheme } from "@/lib/appearance";

const THEME_PAINT_STORAGE_KEY = "flame.theme-paint";
export function rememberThemePaint(theme: string, scheme: Scheme, background: string): void {
  const color = colord(background);
  if (!color.isValid()) return;
  try {
    localStorage.setItem(
      THEME_PAINT_STORAGE_KEY,
      JSON.stringify({ theme, scheme, background: color.toHex() }),
    );
  } catch (error) {
    if (
      error instanceof DOMException &&
      ["SecurityError", "QuotaExceededError"].includes(error.name)
    )
      return;
    throw error;
  }
}
