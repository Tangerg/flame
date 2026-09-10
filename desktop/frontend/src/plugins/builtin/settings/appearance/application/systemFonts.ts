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
 * The families the picker offers: installed on this machine, and able to keep the design's
 * promise about numbers. Memoised per `mono` so the picker does not re-probe on every render.
 *
 * The two filters are ONE rule — the picker only offers a face the product can honour — and
 * they run in this order because asking whether a missing family has tabular figures measures
 * the fallback instead. That the second filter is a no-op for monospace is a property of
 * monospace rather than a special case worth writing here.
 *
 * An empty result is a usable picker: the control's own default entry resolves to the native
 * system stack, which is what the app runs on before anyone overrides anything.
 */
export function useSystemFonts(mono: boolean): string[] {
  return useMemo(() => {
    const candidates = mono ? CANDIDATE_CODE_FONTS : CANDIDATE_UI_FONTS;
    const { isAvailable, hasTabularFigures } = fontAvailability();
    return candidates.filter((family) => isAvailable(family) && hasTabularFigures(family));
  }, [mono]);
}
