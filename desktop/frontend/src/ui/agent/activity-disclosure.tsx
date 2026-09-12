import * as stylex from "@stylexjs/stylex";
import { reveal } from "@/ui/atoms/reveal";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { Children, useId } from "react";
import { cn } from "@/lib/classNames";
import { color, leading, motion, radius, space, surface, type } from "@/styles/tokens.stylex";
import { Collapsible } from "@/ui/atoms/collapsible";
import { Pressable } from "@/ui/atoms/pressable";
import { ProgressBar } from "@/ui/atoms/progress-bar";
import { Icon, type IconName } from "@/ui/icons";
import { toneInk } from "@/ui/atoms/tone-ink";
import { chevron } from "@/ui/atoms/chevron";

type ActivityTone = "neutral" | "warning" | "negative";

/**
 *   line  Work-narrative activity; disclosed material owns any terminal/diff surface.
 *   card  A composite product with a narrative of its own, such as a delegated Run.
 *
 * Stated by every caller rather than defaulted: the atom cannot infer which of the two a row is.
 */
type ActivityShell = "line" | "card";

type ActivityLeading = { icon: IconName; leading?: never } | { icon?: never; leading: ReactNode };

const styles = stylex.create({
  frame: { minWidth: 0, overflow: "clip" },
  frameLine: { borderRadius: radius.sm },
  frameCard: { borderRadius: radius.card, backgroundColor: surface.card },
  header: { display: "flex", minWidth: 0, alignItems: "center" },
  // A header that stays put while its own disclosure scrolls under it has to be opaque, and
  // opaque against whichever plane it is sitting on.
  stuck: { position: "sticky", top: 0, zIndex: 1 },
  stuckLine: { backgroundColor: surface.canvas },
  stuckCard: { backgroundColor: surface.card },
  trigger: { display: "flex", minWidth: 0, flex: 1, alignItems: "center", textAlign: "left" },
  triggerLine: {
    gap: space.s1_5,
    paddingBlock: space.s0_5,
    paddingRight: 0,
    paddingLeft: 0,
    minHeight: space.s5,
  },
  triggerCard: {
    gap: space.s3,
    paddingBlock: space.s1_5,
    paddingRight: space.s3,
    paddingLeft: space.s3,
    minHeight: space.s8,
    backgroundColor: { default: null, ":hover": surface.hover },
    transitionProperty: "color, background-color",
    transitionDuration: motion.color,
  },
  mark: { display: "grid", flexShrink: 0, placeItems: "center", height: space.s4 },
  markLine: { width: space.s4 },
  markCard: { width: space.s5 },
  markFramed: { height: space.s5, borderRadius: radius.sm },
  trayNeutral: { backgroundColor: surface.surface2 },
  trayWarning: { backgroundColor: surface.warningBadge },
  trayNegative: { backgroundColor: surface.negativeBadge },
  // The row's NAME. What keeps it from reaching zero is not a floor here but `trailing` below
  // being shrinkable — measured in German, `Subagent` came out 1.1px wide while it was the only
  // item in the row that could give way. A `min-width` floor also fixed it and was dropped: it
  // widens every label shorter than the floor, which moved rows that were never squeezed.
  label: {
    display: "flex",
    minWidth: 0,
    flexShrink: 1,
    alignItems: "center",
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    color: "var(--row-ink)",
  },
  detail: {
    display: "flex",
    minWidth: 0,
    flex: 1,
    alignItems: "center",
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    lineHeight: leading.snug,
    color: "var(--row-ink)",
  },
  spacer: { minWidth: 0, flex: 1 },
  /**
   * The row's ANNOTATION, and therefore the part that yields first.
   *
   * This was `flex-shrink: 0` while holding text a locale decides the length of — a status
   * phrase plus a step count — so a long one took the row and starved the name beside it.
   * Shrinking is weighted by base size, which is exactly the right distribution here: the
   * oversized trailing gives up most of any deficit and a short one gives up almost nothing.
   *
   * Deliberately NOT `overflow: hidden`, which is the obvious companion to shrinking and was
   * measured to cost more than it buys: a `StatusDot` paints a pulse OUTSIDE its own box, and
   * clipping the slot cut the halo off every running row. The children shrink with the slot
   * and keep their text, so there is nothing here that needs clipping.
   */
  trailing: {
    display: "flex",
    minWidth: 0,
    flexShrink: 1,
    alignItems: "center",
    gap: space.s1_5,
    fontFamily: "var(--font-mono)",
    color: color.fgFaint,
  },
  // The chevron answers two different states, so two publishers and one reader.
  //
  // The HEADER publishes on hover, because hovering the actions beside the trigger has to
  // reveal it too. The TRIGGER publishes on `:focus-visible` only — no `default`, so outside
  // focus the property is simply not set here and the header's value inherits through. DOM
  // focus outlives the pointer, which is why this cannot be the header's `:focus-within`: a
  // row clicked shut kept its chevron lit while every identical row beside it stayed blank.
  //
  // This replaces a pair of `group/` markers and, with them, the `:has(:focus-visible)`
  // workaround they existed to avoid — the chevron is INSIDE the trigger, so an inherited
  // custom property reaches it and no sibling selector is needed.
  headerPublishes: {
    "--chevron": { default: "0", ":hover": "1" },
    // The summary lifts to full ink with the row, so the label and the detail read the same
    // channel rather than each watching an ancestor.
    "--row-ink": { default: color.fgMuted, ":hover": color.fg },
  },
  triggerPublishes: { "--chevron": { default: null, ":focus-visible": "1" } },
  // Decorative: the whole header is the button, so clicks pass through the chevron at every
  // reveal state rather than landing on something nobody aimed at.
  chevron: {
    pointerEvents: "none",
    display: "flex",
    flexShrink: 0,
    color: color.fgFaint,
    transitionProperty: "rotate, opacity",
    transitionDuration: motion.fast,
    // A device with no pointer can never hover, so the mark it would have revealed is simply
    // shown. The `[data-reveal]` rule in `globals.css` said this for everyone and can no
    // longer outrank a generated one.
    opacity: { default: "var(--chevron, 1)", "@media (hover: none)": 1 },
  },
  chevronOpen: { opacity: 1 },
  actions: {
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s0_5,
    paddingLeft: space.s0_5,
    paddingRight: space.s2,
  },
  // Only the SIDES: block padding varies by call site, so it is not a default here.
  bodyLine: { paddingRight: 0 },
  bodyCard: { paddingInline: space.s3 },
  bodyRows: { paddingBlock: space.s1_5 },
});

const TRAY_TONE = {
  neutral: styles.trayNeutral,
  warning: styles.trayWarning,
  negative: styles.trayNegative,
} as const;

type AgentActivityDisclosureProps = Omit<ComponentPropsWithoutRef<"div">, "children"> &
  ActivityLeading & {
    label: ReactNode;
    detail?: ReactNode;
    trailing?: ReactNode;
    actions?: ReactNode;
    open: boolean;
    onToggle: () => void;
    stickyHeader?: boolean;
    progress?: { value: number; label: string };
    toggleLabel?: string;
    tone?: ActivityTone;
    shell: ActivityShell;
    children: ReactNode;
    /** The standing inset for a disclosure whose body is a list of rows. */
    contentInset?: "rows";
    contentClassName?: string;
  };

export function AgentActivityDisclosure({
  icon,
  leading,
  label,
  detail,
  trailing,
  actions,
  open,
  onToggle,
  stickyHeader,
  progress,
  toggleLabel,
  tone = "neutral",
  shell,
  children,
  className,
  contentInset,
  contentClassName,
  ...props
}: AgentActivityDisclosureProps) {
  const line = shell === "line";
  const triggerId = useId();
  const panelId = useId();
  const framed = shell !== "line" && icon !== undefined;

  return (
    <div
      {...props}
      data-slot="agent-activity-disclosure"
      data-tone={tone}
      data-shell={shell}
      className={cn(
        stylex.props(styles.frame, line ? styles.frameLine : styles.frameCard).className,
        className,
      )}
    >
      <div
        data-slot="agent-activity-header"
        // Whether this header outlives its own scroll is a decision, so it is said out loud
        // rather than left to whichever class happened to carry the positioning.
        data-sticky={stickyHeader ? "" : undefined}
        // Publishes the reveal channel for the actions the card hangs here, and the chevron's
        // own channel beside it.
        className={cn(
          stylex.props(
            reveal.host,
            styles.header,
            styles.headerPublishes,
            stickyHeader && [styles.stuck, line ? styles.stuckLine : styles.stuckCard],
          ).className,
        )}
      >
        <Pressable
          id={triggerId}
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          aria-label={toggleLabel}
          // The trigger fills the disclosure, which clips, so an outward ring is cut on
          // three sides.
          data-focus-inset=""
          onClick={onToggle}
          className={cn(
            stylex.props(
              styles.trigger,
              styles.triggerPublishes,
              line ? styles.triggerLine : styles.triggerCard,
            ).className,
          )}
        >
          <span
            aria-hidden
            data-slot="agent-activity-mark"
            // The two decisions this mark makes, said out loud: whether it wears a tray, and
            // which tone. They drive the styles above and they are what a test can hold onto —
            // a generated class name is not a contract.
            data-framed={framed ? "" : undefined}
            data-tone={tone}
            {...stylex.props(
              styles.mark,
              line ? styles.markLine : styles.markCard,
              framed && [styles.markFramed, TRAY_TONE[tone]],
              toneInk[tone],
            )}
          >
            {leading ?? (icon ? <Icon name={icon} size="xs" /> : null)}
          </span>
          <span data-slot="agent-activity-label" {...stylex.props(styles.label, type.uiSm)}>
            {label}
          </span>
          {/* The slot is always here, empty or not: `flex-1` lived on the detail, so a row
              without one stopped pushing its trailing to the right and put its status
              wherever the label happened to end. One row of a four-row fan-out did that,
              440px left of the three beside it, which is the column a reader scans to find
              the sub-agent that failed. Same element either way, so the gap count — and
              every row that does have a detail — is unchanged. */}
          {detail != null ? (
            <span {...stylex.props(styles.detail, type.uiSm)}>{detail}</span>
          ) : (
            <span aria-hidden {...stylex.props(styles.spacer)} />
          )}
          {trailing != null && (
            <span {...stylex.props(styles.trailing, type.ui2xs)}>{trailing}</span>
          )}
          <span
            aria-hidden
            data-slot="agent-activity-chevron"
            data-open={open ? "" : undefined}
            data-reveal="hover"
            {...stylex.props(styles.chevron, open ? styles.chevronOpen : chevron.shut)}
          >
            <Icon name="chevron-down" size="xs" />
          </span>
        </Pressable>
        {Children.count(actions) > 0 && <div {...stylex.props(styles.actions)}>{actions}</div>}
      </div>
      {progress && <ProgressBar value={progress.value} label={progress.label} weight="seam" />}
      <Collapsible open={open}>
        <div
          id={panelId}
          role="region"
          aria-labelledby={triggerId}
          className={cn(
            stylex.props(
              line ? styles.bodyLine : styles.bodyCard,
              contentInset === "rows" && styles.bodyRows,
            ).className,
            contentClassName,
          )}
        >
          {children}
        </div>
      </Collapsible>
    </div>
  );
}
