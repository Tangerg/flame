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
  // Corner ladder: tags, then controls/rows, then cards, then floating surfaces.
  "style-shape-2xs": "2px",
  "style-shape-xs": "4px",
  "style-shape-sm": "6px",
  "style-shape-md": "8px",
  "style-shape-lg": "10px",
  "style-shape-xl": "12px",
  // Deliberately ROUNDER than anything that floats: on the floating rung, the one surface
  // you type into reads as another panel. One value for both rest and wrapped states —
  // moving between them needs the wrapped line count, which is a measurement, not a style.
  "style-shape-composer": "20px",
  // The widest corner in the language: a bubble is small and quoted, and at the card
  // radius a narrow one reads as a cropped card.
  "style-shape-bubble": "16px",
  "button-radius": "var(--shape-sm)",
  "field-radius": "var(--shape-md)",
  "segmented-radius": "var(--shape-md)",
  "segment-radius": "var(--shape-sm)",
  "surface-card-radius": "var(--shape-md)",
  // ABOVE the card's rung: a row is a full-bleed target whose corner exists only to shape
  // the wash sliding under the cursor, and at the card rung that wash reads as a stack of
  // stubby cards.
  "row-radius": "var(--shape-lg)",
  "floating-panel-radius": "var(--shape-xl)",
  "floating-tip-radius": "var(--shape-sm)",
  // A tab is the top of the panel under it, so it takes the surface rung, not a control's.
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
  // Exactly one device pixel on a 2x panel, which is what makes a hairline read as
  // weightless. At 1px the composer becomes the heaviest edge on the screen.
  "composer-edge-width": "0.5px",

  // Ride the SAME step as the surface ladder, so a hovered row is exactly one rung of
  // separation in either scheme and at any contrast setting. Fixed alphas cannot.
  "wash-hover": "color-mix(in srgb, var(--color-text) calc(var(--depth-step) * 0.75), transparent)",
  "wash-selected": "color-mix(in srgb, var(--color-text) var(--depth-step), transparent)",

  // Three opaque materials plus a one-device-pixel seam over each step. BOTH halves are
  // load-bearing: the value delta alone measures too small to read, and a line alone draws
  // the columns as a wireframe of pasted rectangles.
  "app-drawer-surface": "var(--color-surface)",
  "app-content-surface": "var(--color-bg)",
  // Transparent because a chrome bar is the TOP of its column, not a third material
  // stacked on it.
  "app-header-surface": "transparent",
  "app-dock-surface": "color-mix(in oklab, var(--color-bg) 25%, var(--color-surface))",
  // The one bar that is NOT the ground of what it labels: the panel's ground has to reach
  // up into the selected tab, which only reads if the strip itself sits a step back.
  "app-dock-tabstrip-surface":
    "color-mix(in oklab, var(--color-text) calc(var(--depth-step) * 0.75), var(--app-dock-surface))",
  // Identity with the panel's ground IS the tab metaphor; a style wanting a chip or an
  // underline retargets this ONE value (see `data-control-treatment`).
  "dock-tab-active-surface": "var(--app-dock-surface)",
  "app-card-surface": "var(--color-elevated)",
  // Translucent so the composer picks up what passes underneath — that, not the ring, is
  // what makes the ring read as the edge of glass. A flat style spells this opaque.
  "app-composer-surface": "color-mix(in oklab, var(--app-content-surface) 86%, transparent)",
  "composer-backdrop": "blur(20px) saturate(1.4)",
  "app-composer-tray-surface": "color-mix(in oklab, var(--app-content-surface) 70%, transparent)",
  // The rear plane BEHIND the glass composer: one opaque ink step so the composer can
  // overlap it cleanly.
  "app-composer-project-tray-surface":
    "color-mix(in srgb, var(--color-text) 4%, var(--app-content-surface))",
  "composer-tray-backdrop": "blur(8px)",
  "composer-tray-edge-color": "color-mix(in oklab, var(--color-border) 80%, transparent)",
  // A HINT of the page, not the page: transparency and blur are one recipe and must move
  // together, which is why both live in tokens rather than half here and half in a utility
  // class. Three times more see-through and five times more blurred is the glassy look
  // this design language forbids everywhere else.
  "app-floating-surface": "color-mix(in oklab, var(--app-content-surface) 90%, transparent)",
  "floating-backdrop": "blur(8px) saturate(1.4)",
  // Drawn INSIDE the plane: it outranks the drawer on z-index, so the drawer sliding under
  // it cannot draw the seam from outside. GEOMETRY ONLY — these are declared on :root, so a
  // var() here resolves there, where the shell's live boundary variables do not exist; the
  // shell names the colour on the element. Half a pixel rather than a cast, which would
  // spread onto the reading plane and read as pressing down on the document.
  "app-card-edge": "inset 0.5px 0 0 0",
  // `-end` is the mirror, for a pane docked to the other side.
  "app-pane-split": "inset 0.5px 0 0 0",
  "app-pane-split-end": "inset -0.5px 0 0 0",
  "app-header-edge": "inset 0 -0.5px 0 0",
  // The seam's second half, and it falls on the CHROME, not on the page: a gradient on the
  // panel reads soft, the same gradient on the reading plane reads as pressure.
  "app-pane-wash": "inset 12px 0 12px -6px",
  "app-pane-wash-end": "inset -12px 0 12px -6px",
  // The one place an optical ring still earns its pixel: a floating panel has no value
  // delta to lean on, because it can land over anything.
  "seam-line": "var(--color-border-soft)",

  // Flush chrome casts nothing; only surfaces that genuinely leave the plane carry depth.
  "shadow-control": "none",
  // Two layers, and the first has NO offset on purpose: an ambient bloom on all four sides
  // is what makes a half-pixel ring legible at all — with the drop alone, the top and side
  // edges measure 1-2 levels off the plane.
  // Depth only; the ring is drawn from `--composer-edge-width` at the callsite, where the
  // colour can answer focus.
  "shadow-composer-depth":
    "0 8px 16px -4px color-mix(in oklab, var(--shadow-cast) 60%, transparent), 0 0 26px -8px var(--shadow-cast)",
  // NOT named `shadow-sm/lg/xl`: those are Tailwind's own theme keys.
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
  disclosureMs: 220,
  slowMs: 360,
  // Longer than the curve it replaced, and more immediate: it reaches 54% / 90% / 99% at
  // 100 / 200 / 300ms, then spends the rest of its clock on a sub-pixel settle. A uniform
  // 300ms curve was only halfway across at 150ms, so the text reflowed at visible speed
  // for the whole gesture and read as drag.
  drawerMs: 500,
  easeOut: [0.22, 1, 0.36, 1],
  easeInOut: [0.45, 0, 0.55, 1],
  easeEmphasized: [0.16, 1, 0.3, 1],
  // A sampled spring published as native CSS `linear()`, not a fitted cubic: it preserves
  // the overshoot, and native transition reversal keeps an interrupted gesture continuous
  // without an animation-frame owner in React. `--ease-drawer` in globals.css mirrors these
  // for the frame before the visual style is published.
  drawerProgress: [
    0, 0.06981, 0.21761, 0.38345, 0.53716, 0.66615, 0.76765, 0.84375, 0.89859, 0.93672, 0.96233,
    0.97894, 0.98929, 0.99544, 0.99887, 1.00061, 1.00135, 1.00152, 1.00142, 1.00119, 1,
  ],
  pressScale: 0.98,
} as const;

export function visualStyleTokens(overrides: Partial<VisualStyleTokens>): VisualStyleTokens {
  return { ...WORKBENCH_TOKENS, ...overrides };
}

/**
 * A style's motion as the custom properties the chrome reads. Beside the other token
 * projections rather than inside the painter: naming a property is this module's job, and
 * applying one is the painter's. It also puts the projection where `globals.css`'s mirror
 * of it can be compared against it.
 */
export function visualStyleMotionTokens(motion: VisualStyleMotion): Record<string, string> {
  const bezier = (value: readonly [number, number, number, number]) =>
    `cubic-bezier(${value.join(", ")})`;
  return {
    "dur-instant-base": `${motion.instantMs}ms`,
    "dur-fast-base": `${motion.fastMs}ms`,
    "dur-med-base": `${motion.mediumMs}ms`,
    "dur-disclosure-base": `${motion.disclosureMs}ms`,
    "dur-slow-base": `${motion.slowMs}ms`,
    "dur-drawer-base": `${motion.drawerMs}ms`,
    "ease-out": bezier(motion.easeOut),
    "ease-in-out": bezier(motion.easeInOut),
    "ease-emphasized": bezier(motion.easeEmphasized),
    "ease-drawer": `linear(${motion.drawerProgress.join(", ")})`,
    "press-scale": String(motion.pressScale),
  };
}
