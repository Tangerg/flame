import * as stylex from "@stylexjs/stylex";

export const color = stylex.defineVars({
  fg: "var(--color-text)",
  fgSoft: "var(--color-text-soft)",
  fgMuted: "var(--color-text-muted)",
  fgFaint: "var(--color-text-faint)",
  accent: "var(--color-accent)",
  onAccent: "var(--color-text-on-accent)",
  ctaText: "var(--color-cta-text)",
  onMedia: "var(--color-on-media)",
  negative: "var(--color-negative)",
  warning: "var(--color-warning)",
  success: "var(--color-success)",
  info: "var(--color-info)",
});

export const motion = stylex.defineVars({
  fast: "var(--dur-fast)",
  instant: "var(--dur-instant)",
  med: "var(--dur-med)",
  riseIn: "var(--animate-rise-in)",
  color: "var(--dur-color)",
  easeOut: "var(--ease-out)",
  easeState: "var(--ease-state)",
  shimmer: "var(--animate-shimmer)",
  sweep: "var(--animate-sweep)",
  pulseDot: "var(--animate-pulse-dot)",
  spin: "var(--animate-spin)",
  breathe: "var(--animate-breathe)",
});

export const surface = stylex.defineVars({
  sunken: "var(--color-sunken)",
  sunkenHover: "var(--color-sunken-hover)",
  surface2: "var(--color-surface-2)",
  divider: "var(--color-divider)",
  surface3: "var(--color-surface-3)",
  surface: "var(--color-surface)",
  canvas: "var(--color-bg)",
  floating: "var(--app-floating-surface)",
  scrim: "var(--color-scrim)",
  hover: "var(--wash-hover)",
  ctaFill: "var(--color-cta)",
  ctaHover: "var(--color-cta-hover)",
  mediaScrim: "var(--color-media-scrim)",
  accentWash: "var(--color-accent-wash)",
  negativeWash: "var(--color-negative-wash)",
  infoWash: "var(--color-info-wash)",
  successWash: "var(--color-success-wash)",
  warningWash: "var(--color-warning-wash)",
  joinSeam: "var(--button-join-seam)",
  selected: "var(--wash-selected)",
  selectedHover: "var(--wash-selected-hover)",
  lineSoft: "var(--color-line-soft)",
  mediaField: "var(--color-media-preview)",
  card: "var(--app-card-surface)",
  field: "var(--color-border)",
  fieldStrong: "var(--color-border-soft)",
  fieldFocus: "var(--color-focus-ring)",
  accentBadge: "var(--color-accent-badge)",
  successBadge: "var(--color-success-badge)",
  warningBadge: "var(--color-warning-badge)",
  negativeBadge: "var(--color-negative-badge)",
  infoBadge: "var(--color-info-badge)",
});

export const radius = stylex.defineVars({
  step2xs: "var(--shape-2xs)",
  xs: "var(--shape-xs)",
  card: "var(--surface-card-radius)",
  bubble: "var(--shape-bubble)",
  sm: "var(--shape-sm)",
  lg: "var(--shape-lg)",
  field: "var(--field-radius)",
  segmented: "var(--segmented-radius)",
  segment: "var(--segment-radius)",
  row: "var(--row-radius)",
  button: "var(--button-radius)",
  floatingPanel: "var(--floating-panel-radius)",
  floatingTip: "var(--floating-tip-radius)",
  composer: "var(--shape-composer)",
  xl: "var(--shape-xl)",
});

export const space = stylex.defineVars({
  s0_5: "calc(var(--spacing) * 0.5)",
  s1: "calc(var(--spacing) * 1)",
  s1_5: "calc(var(--spacing) * 1.5)",
  s2: "calc(var(--spacing) * 2)",
  s2_5: "calc(var(--spacing) * 2.5)",
  s3: "calc(var(--spacing) * 3)",
  s3_5: "calc(var(--spacing) * 3.5)",
  s4: "calc(var(--spacing) * 4)",
  s4_5: "calc(var(--spacing) * 4.5)",
  s5: "calc(var(--spacing) * 5)",
  s6: "calc(var(--spacing) * 6)",
  s7: "calc(var(--spacing) * 7)",
  s8: "calc(var(--spacing) * 8)",
  s9: "calc(var(--spacing) * 9)",
  s10: "calc(var(--spacing) * 10)",
  s11: "calc(var(--spacing) * 11)",
  s12: "calc(var(--spacing) * 12)",
  s14: "calc(var(--spacing) * 14)",
  s16: "calc(var(--spacing) * 16)",
  s24: "calc(var(--spacing) * 24)",
});

export const weight = stylex.defineVars({
  regular: "var(--fw-regular)",
  medium: "var(--fw-medium)",
  semibold: "var(--fw-semibold)",
});

export const leading = stylex.defineVars({
  body: "var(--leading-body)",
  relaxed: "var(--leading-relaxed)",
  prose: "var(--leading-prose)",
  snug: "var(--leading-snug)",
  tight: "var(--leading-tight)",
});

export const corner = stylex.create({
  pill: { borderRadius: "var(--shape-pill)", "corner-shape": "round" },
  bubble: { borderRadius: "var(--shape-bubble)", "corner-shape": "round" },
});

export const type = stylex.create({
  ui2xs: { fontSize: "var(--fs-ui-2xs)", letterSpacing: "var(--text-ui-2xs--letter-spacing)" },
  uiXs: { fontSize: "var(--fs-ui-xs)", letterSpacing: "var(--tracking-ui)" },
  uiSm: { fontSize: "var(--fs-ui-sm)", letterSpacing: "var(--tracking-ui)" },
  uiMd: { fontSize: "var(--fs-ui-md)", letterSpacing: "var(--tracking-ui)" },
  displaySm: {
    fontSize: "var(--fs-display-sm)",
    letterSpacing: "var(--tracking-ui)",
  },
  displayMd: {
    fontSize: "var(--fs-display-md)",
    letterSpacing: "var(--tracking-display)",
    lineHeight: "var(--text-display-md--line-height)",
  },
  code: { fontSize: "var(--fs-code)", letterSpacing: "var(--text-code--letter-spacing)" },
  prose: { fontSize: "var(--fs-prose)", letterSpacing: "var(--tracking-ui)" },
});

export const face = stylex.create({
  text: { fontFamily: "var(--font-sans)" },
  mono: { fontFamily: "var(--font-mono)", letterSpacing: 0 },
});
