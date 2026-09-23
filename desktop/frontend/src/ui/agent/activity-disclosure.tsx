import * as stylex from "@stylexjs/stylex";
import { reveal } from "@/ui/atoms/reveal";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { Children, useId } from "react";
import { cn } from "@/lib/classNames";
import { color, leading, motion, radius, space, surface, type } from "@/styles/tokens.stylex";
import { Collapsible, useDisclosedContent } from "@/ui/atoms/collapsible";
import { Pressable } from "@/ui/atoms/pressable";
import { ProgressBar } from "@/ui/atoms/progress-bar";
import { Icon, type IconName } from "@/ui/icons";
import { toneInk } from "@/ui/atoms/tone-ink";
import { chevron } from "@/ui/atoms/chevron";

type ActivityTone = "neutral" | "warning" | "negative";

type ActivityShell = "line" | "card";

type ActivityLeading = { icon: IconName; leading?: never } | { icon?: never; leading: ReactNode };

const styles = stylex.create({
  frame: { minWidth: 0, overflow: "clip" },
  frameLine: { borderRadius: radius.sm },
  frameCard: {
    borderRadius: radius.card,
    backgroundColor: surface.card,
    borderWidth: "var(--control-edge-width)",
    borderStyle: "solid",
    borderColor: surface.field,
  },
  header: { display: "flex", minWidth: 0, alignItems: "center" },
  stuck: { position: "sticky", top: 0, zIndex: 1 },
  stuckLine: { backgroundColor: surface.canvas },
  stuckCard: { backgroundColor: surface.card },
  trigger: {
    display: "flex",
    minWidth: 0,
    flex: 1,
    alignItems: "center",
    textAlign: "left",
    lineHeight: leading.body,
  },
  triggerLine: {
    gap: space.s1_5,
    paddingBlock: space.s0_5,
    paddingRight: 0,
    paddingLeft: 0,
    minHeight: "var(--control-height-sm)",
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
    transitionTimingFunction: motion.easeState,
  },
  mark: { display: "grid", flexShrink: 0, placeItems: "center", height: space.s4 },
  markLine: { width: space.s4 },
  markCard: { width: space.s5 },
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
    minWidth: 0,
    flexShrink: 1,
    alignItems: "center",
    gap: space.s1_5,
    fontFamily: "var(--font-mono)",
    color: color.fgFaint,
  },
  headerPublishes: {
    "--chevron": { default: "0", ":hover": "1" },
    "--row-ink": { default: color.fgMuted, ":hover": color.fg },
  },
  triggerPublishes: { "--chevron": { default: null, ":focus-visible": "1" } },
  chevron: {
    pointerEvents: "none",
    display: "flex",
    flexShrink: 0,
    color: color.fgFaint,
    transitionProperty: "rotate, opacity",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
    opacity: { default: "var(--chevron, 1)", "@media (hover: none)": 1 },
  },
  chevronOpen: { opacity: 1 },
  chevronSlot: { flexShrink: 0, width: "var(--icon-xs)" },
  actions: {
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s0_5,
    paddingLeft: space.s0_5,
    paddingRight: space.s2,
  },
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
    children?: ReactNode;
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
  const disclosed = useDisclosedContent(open);

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
        data-sticky={stickyHeader ? "" : undefined}
        className={cn(
          stylex.props(
            reveal.host,
            styles.header,
            styles.headerPublishes,
            stickyHeader && [styles.stuck, line ? styles.stuckLine : styles.stuckCard],
          ).className,
        )}
      >
        <TriggerShell
          disclosable={children != null}
          id={triggerId}
          panelId={panelId}
          open={open}
          onToggle={onToggle}
          toggleLabel={toggleLabel}
          className={
            stylex.props(
              styles.trigger,
              styles.triggerPublishes,
              line ? styles.triggerLine : styles.triggerCard,
            ).className
          }
        >
          <span
            aria-hidden
            data-slot="agent-activity-mark"
            data-framed={framed ? "" : undefined}
            data-tone={tone}
            {...stylex.props(
              styles.mark,
              line ? styles.markLine : styles.markCard,
              framed && [styles.markFramed, TRAY_TONE[tone]],
              toneInk[tone],
            )}
          >
            {leading ?? (icon ? <Icon name={icon} size="md" /> : null)}
          </span>
          <span data-slot="agent-activity-label" {...stylex.props(styles.label, type.uiMd)}>
            {label}
          </span>
          {detail != null ? (
            <span {...stylex.props(styles.detail, type.uiMd)}>{detail}</span>
          ) : (
            <span aria-hidden {...stylex.props(styles.spacer)} />
          )}
          {trailing != null && (
            <span {...stylex.props(styles.trailing, type.uiXs)}>{trailing}</span>
          )}
          {children != null ? (
            <span
              aria-hidden
              data-slot="agent-activity-chevron"
              data-open={open ? "" : undefined}
              data-reveal="hover"
              {...stylex.props(styles.chevron, open ? styles.chevronOpen : chevron.shut)}
            >
              <Icon name="chevron-down" size="xs" />
            </span>
          ) : (
            <span aria-hidden {...stylex.props(styles.chevronSlot)} />
          )}
        </TriggerShell>
        {Children.count(actions) > 0 && (
          <div data-focus-inset="" {...stylex.props(styles.actions)}>
            {actions}
          </div>
        )}
      </div>
      {progress && <ProgressBar value={progress.value} label={progress.label} weight="seam" />}
      {children != null && (
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
            {disclosed && children}
          </div>
        </Collapsible>
      )}
    </div>
  );
}

function TriggerShell({
  disclosable,
  id,
  panelId,
  open,
  onToggle,
  toggleLabel,
  className,
  children,
}: {
  disclosable: boolean;
  id: string;
  panelId: string;
  open: boolean;
  onToggle: () => void;
  toggleLabel?: string;
  className?: string;
  children: ReactNode;
}) {
  if (!disclosable) {
    return (
      <div id={id} className={className}>
        {children}
      </div>
    );
  }
  return (
    <Pressable
      id={id}
      type="button"
      aria-expanded={open}
      aria-controls={panelId}
      aria-label={toggleLabel}
      data-focus-inset=""
      onClick={onToggle}
      className={className}
    >
      {children}
    </Pressable>
  );
}
