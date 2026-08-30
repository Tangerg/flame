import { configureFontAvailabilityPort } from "../application/ports/fontAvailability";

// `queryLocalFonts()` would enumerate properly but is permission-gated and unsupported in
// WebKit, which Wails ships on macOS — so a curated candidate list filtered through
// `document.fonts.check()` is the portable answer. A missing API counts as "available":
// hiding every font leaves the picker empty, which is worse than one that falls back.
function isAvailable(family: string): boolean {
  if (typeof document === "undefined") return false;
  const check = document.fonts?.check;
  if (typeof check !== "function") return true;
  try {
    return document.fonts.check(`12px "${family}"`);
  } catch {
    return false;
  }
}

export function installBrowserFontAvailability(): () => void {
  return configureFontAvailabilityPort({ isAvailable });
}
