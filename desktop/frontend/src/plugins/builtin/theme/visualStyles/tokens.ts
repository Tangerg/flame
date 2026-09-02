import type { VisualStyleMotion } from "@/lib/appearance";

type VisualStyleTokenName =
  | "style-shape-2xs"
  | "style-shape-xs"
  | "style-shape-sm"
  | "style-shape-md"
  | "style-shape-lg"
  | "style-shape-xl"
  | "style-shape-composer"
  | "style-shape-bubble"
  | "button-radius"
  | "field-radius"
  | "segmented-radius"
  | "segment-radius"
  | "surface-card-radius"
  | "row-radius"
  | "floating-panel-radius"
  | "floating-tip-radius"
  | "dock-tab-radius"
  | "control-height-xs"
  | "control-height-sm"
  | "control-height-md"
  | "control-height-lg"
  | "field-height-sm"
  | "field-height-md"
  | "field-height-lg"
  | "menu-row-height"
  | "dock-tab-height"
  | "surface-header-height"
  | "control-edge-width"
  | "composer-edge-width"
  | "wash-hover"
  | "wash-selected"
  | "app-drawer-surface"
  | "app-content-surface"
  | "app-header-surface"
  | "app-dock-surface"
  | "app-dock-tabstrip-surface"
  | "dock-tab-active-surface"
  | "app-card-surface"
  | "app-composer-surface"
  | "app-composer-tray-surface"
  | "app-composer-project-tray-surface"
  | "app-floating-surface"
  | "composer-backdrop"
  | "composer-tray-backdrop"
  | "composer-tray-edge-color"
  | "floating-backdrop"
  | "app-card-edge"
  | "app-pane-split"
  | "app-pane-split-end"
  | "app-header-edge"
  | "app-pane-wash"
  | "app-pane-wash-end"
  | "seam-line"
  | "shadow-control"
  | "shadow-composer-depth"
  | "shadow-ring"
  | "shadow-raised"
  | "shadow-overlay"
  | "shadow-popover"
  | "shadow-modal"
  | "shadow-well"
  | "shadow-raised-chip"
  | "shadow-surface-card";

export type VisualStyleTokens = Record<VisualStyleTokenName, string>;

export const WORKBENCH_TOKENS: VisualStyleTokens = {
  "style-shape-2xs": "2px",
  "style-shape-xs": "4px",
  "style-shape-sm": "6px",
  "style-shape-md": "8px",
  "style-shape-lg": "10px",
  "style-shape-xl": "12px",
  "style-shape-composer": "20px",
  "style-shape-bubble": "16px",
  "button-radius": "var(--shape-sm)",
  "field-radius": "var(--shape-md)",
  "segmented-radius": "var(--shape-md)",
  "segment-radius": "var(--shape-sm)",
  "surface-card-radius": "var(--shape-md)",
  "row-radius": "var(--shape-lg)",
  "floating-panel-radius": "var(--shape-xl)",
  "floating-tip-radius": "var(--shape-sm)",
  "dock-tab-radius": "var(--shape-lg)",

  // Every height is EVEN so a centred 1px rule never lands on a half pixel.
  "control-height-xs": "22px",
  "control-height-sm": "26px",
  "control-height-md": "30px",
  "control-height-lg": "34px",
  "field-height-sm": "26px",
  "field-height-md": "28px",
  "field-height-lg": "32px",
  "menu-row-height": "30px",
  "dock-tab-height": "28px",
  "surface-header-height": "42px",
  "control-edge-width": "1px",
  // One device pixel on a 2x panel; at 1px the composer becomes the heaviest edge on screen.
  "composer-edge-width": "0.5px",

  "wash-hover": "color-mix(in srgb, var(--color-text) calc(var(--depth-step) * 0.75), transparent)",
  "wash-selected": "color-mix(in srgb, var(--color-text) var(--depth-step), transparent)",

  "app-drawer-surface": "var(--color-surface)",
  "app-content-surface": "var(--color-bg)",
  "app-header-surface": "transparent",
  "app-dock-surface": "color-mix(in oklab, var(--color-bg) 25%, var(--color-surface))",
  "app-dock-tabstrip-surface":
    "color-mix(in oklab, var(--color-text) calc(var(--depth-step) * 0.75), var(--app-dock-surface))",
  "dock-tab-active-surface": "var(--app-dock-surface)",
  "app-card-surface": "var(--color-elevated)",
  "app-composer-surface": "color-mix(in oklab, var(--app-content-surface) 86%, transparent)",
  "composer-backdrop": "blur(20px) saturate(1.4)",
  "app-composer-tray-surface": "color-mix(in oklab, var(--app-content-surface) 70%, transparent)",
  "app-composer-project-tray-surface":
    "color-mix(in srgb, var(--color-text) 4%, var(--app-content-surface))",
  "composer-tray-backdrop": "blur(8px)",
  "composer-tray-edge-color": "color-mix(in oklab, var(--color-border) 80%, transparent)",
  "app-floating-surface": "color-mix(in oklab, var(--app-content-surface) 90%, transparent)",
  "floating-backdrop": "blur(8px) saturate(1.4)",
  // GEOMETRY ONLY: declared on :root, so a var() here resolves there, where the shell's live
  // boundary variables do not exist. The shell names the colour on the element.
  "app-card-edge": "inset 0.5px 0 0 0",
  "app-pane-split": "inset 0.5px 0 0 0",
  "app-pane-split-end": "inset -0.5px 0 0 0",
  "app-header-edge": "inset 0 -0.5px 0 0",
  "app-pane-wash": "inset 12px 0 12px -6px",
  "app-pane-wash-end": "inset -12px 0 12px -6px",
  "seam-line": "var(--color-border-soft)",

  "shadow-control": "none",
  "shadow-composer-depth":
    "0 8px 16px -4px color-mix(in oklab, var(--shadow-cast) 60%, transparent), 0 0 26px -8px var(--shadow-cast)",
  // NOT `shadow-sm/lg/xl` — those are Tailwind's own theme keys.
  "shadow-ring": "0 0 0 0.5px var(--seam-line)",
  "shadow-raised":
    "var(--shadow-ring), 0 1px 2px -1px color-mix(in oklab, var(--shadow-cast) 40%, transparent)",
  "shadow-overlay":
    "var(--shadow-ring), 0 4px 8px -2px color-mix(in oklab, var(--shadow-cast) 50%, transparent)",
  "shadow-popover":
    "var(--shadow-ring), 0 8px 16px -4px color-mix(in oklab, var(--shadow-cast) 60%, transparent)",
  "shadow-modal":
    "var(--shadow-ring), 0 16px 32px -8px color-mix(in oklab, var(--shadow-cast) 95%, transparent)",
  "shadow-well": "none",
  "shadow-raised-chip": "none",
  "shadow-surface-card": "none",
};

export const WORKBENCH_MOTION = {
  instantMs: 80,
  fastMs: 150,
  mediumMs: 200,
  slowMs: 300,
  drawerMs: 500,
  easeOut: [0.22, 1, 0.36, 1],
  // A sampled spring as native `linear()`, not a fitted cubic: it keeps the overshoot, and
  // native reversal keeps an interrupted gesture continuous with no React frame owner.
  drawerProgress: [
    0, 0.06981, 0.21761, 0.38345, 0.53716, 0.66615, 0.76765, 0.84375, 0.89859, 0.93672, 0.96233,
    0.97894, 0.98929, 0.99544, 0.99887, 1.00061, 1.00135, 1.00152, 1.00142, 1.00119, 1,
  ],
  pressScale: 0.98,
} as const;

export function visualStyleTokens(overrides: Partial<VisualStyleTokens>): VisualStyleTokens {
  return { ...WORKBENCH_TOKENS, ...overrides };
}

/** A style's motion as the custom properties the chrome reads. */
export function visualStyleMotionTokens(motion: VisualStyleMotion): Record<string, string> {
  const bezier = (value: readonly [number, number, number, number]) =>
    `cubic-bezier(${value.join(", ")})`;
  return {
    "dur-instant-base": `${motion.instantMs}ms`,
    "dur-fast-base": `${motion.fastMs}ms`,
    "dur-med-base": `${motion.mediumMs}ms`,
    "dur-slow-base": `${motion.slowMs}ms`,
    "dur-drawer-base": `${motion.drawerMs}ms`,
    "ease-out": bezier(motion.easeOut),
    "ease-drawer": `linear(${motion.drawerProgress.join(", ")})`,
    "press-scale": String(motion.pressScale),
  };
}
