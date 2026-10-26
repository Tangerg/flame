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

export interface FontAvailability {
  isAvailable(family: string): boolean;
  hasTabularFigures(family: string): boolean;
}

export function systemFonts(availability: FontAvailability, mono: boolean): string[] {
  const candidates = mono ? CANDIDATE_CODE_FONTS : CANDIDATE_UI_FONTS;
  return candidates.filter(
    (family) => availability.isAvailable(family) && availability.hasTabularFigures(family),
  );
}
