import { useMemo } from "react";
import { fontAvailability } from "./ports/fontAvailability";

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

export function useSystemFonts(mono: boolean): string[] {
  return useMemo(() => {
    const candidates = mono ? CANDIDATE_CODE_FONTS : CANDIDATE_UI_FONTS;
    const { isAvailable, hasTabularFigures } = fontAvailability();
    return candidates.filter((family) => isAvailable(family) && hasTabularFigures(family));
  }, [mono]);
}
