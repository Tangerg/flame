import { formatHex, formatHex8, rgbColor } from "@/lib/color";
import { type Scheme } from "@/lib/appearance";
import { THEME_PAINT_STORAGE_KEY } from "@/features/theme/kit/documentTheme";

export function rememberThemePaint(
  storage: () => Storage,
  theme: string,
  scheme: Scheme,
  background: string,
): void {
  const color = rgbColor(background);
  if (!color) return;
  writeStorage(() =>
    storage().setItem(
      THEME_PAINT_STORAGE_KEY,
      JSON.stringify({
        theme,
        scheme,
        background:
          color.alpha !== undefined && color.alpha < 1 ? formatHex8(color) : formatHex(color),
      }),
    ),
  );
}

export function forgetThemePaint(storage: () => Storage): void {
  writeStorage(() => storage().removeItem(THEME_PAINT_STORAGE_KEY));
}

function writeStorage(write: () => void): void {
  try {
    write();
  } catch (error) {
    if (
      error instanceof DOMException &&
      ["SecurityError", "QuotaExceededError"].includes(error.name)
    )
      return;
    throw error;
  }
}
