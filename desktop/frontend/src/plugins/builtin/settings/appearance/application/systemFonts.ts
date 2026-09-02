// A curated cross-platform candidate list; whether a family is installed is asked through
// the font-availability port, which explains why enumeration is not an option. The app
// bundles no webfont — the default is the OS stack and this picker is an override only.

import { useMemo } from "react";
import { fontAvailability } from "./ports/fontAvailability";

// Sans-serif / proportional candidates. Order is "best Mac default → wide
// availability"; the empty default ("") in `useAppearanceStore` already resolves to the
// native system stack, so these are opt-in overrides.
const CANDIDATE_UI_FONTS = [
  "SF Pro Text",
  "SF Pro Display",
  "Inter",
  "Helvetica Neue",
  "Segoe UI",
  "Roboto",
  "Ubuntu",
  "Cantarell",
  "Arial",
];

const CANDIDATE_CODE_FONTS = [
  "SF Mono",
  "Menlo",
  "JetBrains Mono",
  "Fira Code",
  "Cascadia Code",
  "Cascadia Mono",
  "Monaco",
  "Consolas",
  "Source Code Pro",
  "Hack",
  "IBM Plex Mono",
  "DejaVu Sans Mono",
];

/**
 * The list of font families to show in the picker, filtered down to
 * what's actually installed on the user's machine. Memoised per `mono`
 * so the picker doesn't re-detect on every render.
 */
export function useSystemFonts(mono: boolean): string[] {
  return useMemo(() => {
    const candidates = mono ? CANDIDATE_CODE_FONTS : CANDIDATE_UI_FONTS;
    const { isAvailable } = fontAvailability();
    return candidates.filter((family) => isAvailable(family));
  }, [mono]);
}
