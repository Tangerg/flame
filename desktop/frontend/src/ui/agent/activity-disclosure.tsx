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
 * Stated by every caller rather than defaulted: which of the two a row is, is the whole of
 * the difference between a glance and a product, and the atom cannot infer it.
 */
type ActivityShell = "line" | "card";

type ActivityLeading = { icon: IconName; leading?: never } | { icon?: never; leading: ReactNode };

const styles = stylex.create({
  frame: { minWidth: 0, overflow: "clip" },
  // A line is a row in the narrative and takes the small corner; a card is a product of its
  // own and wears the card plane.
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
  // The gutter the glyph sits in: a line's is the glyph's own box, a card's is one step wider
  // because the card's rows have to align down a column.
  mark: { display: "grid", flexShrink: 0, placeItems: "center", height: space.s4 },
  markLine: { width: space.s4 },
  markCard: { width: space.s5 },
  // A framed mark is a plate the glyph sits on, so it is taller and takes a corner and a wash.
  markFramed: { height: space.s5, borderRadius: radius.sm },
  trayNeutral: { backgroundColor: surface.surface2 },
  trayWarning: { backgroundColor: surface.warningBadge },
  trayNegative: { backgroundColor: surface.negativeBadge },
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
  trailing: {
    display: "flex",
    flexShrink: 0,
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
  // Only the SIDES. How much room the disclosed material needs above and below it depends on
  // what it is — a reasoning quote sits tighter under its header than a question's choices do —
  // and two of the six call sites already said so with a `pt`/`pb` that tailwind-merge let
  // through. A default that a third of its callers disagree with is not a default.
  bodyLine: { paddingRight: 0 },
  bodyCard: { paddingInline: space.s3 },
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
            stylex.props(line ? styles.bodyLine : styles.bodyCard).className,
            contentClassName,
          )}
        >
          {children}
        </div>
      </Collapsible>
    </div>
  );
}
