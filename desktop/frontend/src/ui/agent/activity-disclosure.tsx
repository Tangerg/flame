import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { Children, useId } from "react";
import type { ActivityShell } from "@/lib/activityShell";
import { cn } from "@/lib/classNames";
import { Collapsible } from "@/ui/atoms/collapsible";
import { Pressable } from "@/ui/atoms/pressable";
import { ProgressBar } from "@/ui/atoms/progress-bar";
import { Icon, type IconName } from "@/ui/icons";

type ActivityTone = "neutral" | "warning" | "negative";

type ActivityLeading = { icon: IconName; leading?: never } | { icon?: never; leading: ReactNode };

// One width for every row's mark: varying it moves labels out of alignment between rows.
const GUTTER = { cardSlot: "w-5" } as const;

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
    shell?: ActivityShell;
    children: ReactNode;
    contentClassName?: string;
  };

const TONE_CLASS: Record<ActivityTone, string> = {
  neutral: "text-fg-muted",
  warning: "text-warning",
  negative: "text-negative",
};

const FLAG_EDGE_CLASS: Record<ActivityTone, string> = {
  neutral: "border-field",
  warning: "border-warning-edge",
  negative: "border-negative-edge",
};

const TRAY_CLASS: Record<ActivityTone, string> = {
  neutral: "bg-surface-2",
  warning: "bg-warning-badge",
  negative: "bg-negative-badge",
};

/**
 * Shared disclosure grammar for tool calls, reasoning, delegated Runs and plan progress:
 * `shell` declares how much plane the row claims, `tone` declares what state it is in.
 * Owns geometry and disclosure accessibility only — domain state stays with callers.
 */
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
  shell = "card",
  children,
  className,
  contentClassName,
  ...props
}: AgentActivityDisclosureProps) {
  const triggerId = useId();
  const panelId = useId();
  // A caller-supplied `leading` owns its whole box; framing it would nest a mark in a mark.
  const framed = shell !== "line" && icon !== undefined;

  return (
    <div
      {...props}
      data-slot="agent-activity-disclosure"
      data-tone={tone}
      data-shell={shell}
      className={cn(
        // No outer margin: spacing against the previous row depends on what that row
        // was, which only the sequence walker knows (see renderUnitRhythm).
        // `clip`, not `hidden` — `hidden` makes this a scroll container, which becomes
        // the scrollport `stickyHeader` below positions against and never scrolls.
        "min-w-0 overflow-clip",
        shell === "line"
          ? "rounded-[var(--shape-sm)]"
          : "rounded-[var(--surface-card-radius)] bg-card",
        shell === "flagged" && `border ${FLAG_EDGE_CLASS[tone]}`,
        className,
      )}
    >
      <div
        className={cn(
          "group/activity-header flex min-w-0 items-center",
          // The fill follows the SHELL: a stuck header must hide the rows travelling
          // under it, and only the shell knows what ground it sits on.
          stickyHeader && ["sticky top-0 z-1", shell === "line" ? "bg-canvas" : "bg-card"],
        )}
      >
        <Pressable
          id={triggerId}
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          aria-label={toggleLabel}
          onClick={onToggle}
          className={cn(
            "flex min-w-0 flex-1 items-center text-left",
            shell === "line" ? "gap-1.5 py-0.5 pr-0" : "gap-3 py-1.5 pr-3",
            shell === "line" ? "pl-0" : "pl-3",
            shell !== "line" && "transition-colors duration-[var(--dur-color)] hover:bg-hover",
            shell === "line" ? "min-h-5" : "min-h-8",
          )}
        >
          <span
            aria-hidden
            data-slot="agent-activity-mark"
            className={cn(
              "grid shrink-0 place-items-center",
              shell === "line" ? "h-4 w-4" : GUTTER.cardSlot,
              framed ? `h-5 rounded-[var(--shape-sm)] ${TRAY_CLASS[tone]}` : "h-4",
              shell === "line" && tone === "neutral" ? "text-fg-faint" : TONE_CLASS[tone],
            )}
          >
            {leading ?? (icon ? <Icon name={icon} size="xs" /> : null)}
          </span>
          {/* `shrink`, not `shrink-0`: `truncate` needs a box allowed to shrink, or a
              long label runs past the card's corner and is cut with no ellipsis. */}
          <span
            data-slot="agent-activity-label"
            className="flex min-w-0 shrink items-center overflow-hidden text-ellipsis whitespace-nowrap text-ui-sm text-fg-muted group-hover/activity-header:text-fg"
          >
            {label}
          </span>
          {detail != null && (
            <span className="flex min-w-0 flex-1 items-center overflow-hidden text-ellipsis whitespace-nowrap text-ui-sm leading-snug text-fg-muted group-hover/activity-header:text-fg">
              {detail}
            </span>
          )}
          {trailing != null && (
            <span className="flex shrink-0 items-center gap-1.5 font-mono text-ui-2xs text-fg-faint tabular-nums">
              {trailing}
            </span>
          )}
          <span
            aria-hidden
            data-slot="agent-activity-chevron"
            className={cn(
              "flex shrink-0 text-fg-faint transition-[transform,opacity] duration-[var(--dur-fast)] group-focus-within/activity-header:opacity-100 group-hover/activity-header:opacity-100",
              open ? "opacity-100" : "-rotate-90 opacity-0",
            )}
          >
            <Icon name="chevron-down" size="xs" />
          </span>
        </Pressable>
        {/* `Children.count`, not a null check: an empty child list is ordinary input and
            is not "no rail" — rendered anyway it leaves a 2px stub on every settled row. */}
        {Children.count(actions) > 0 && (
          <div className="flex shrink-0 items-center gap-0.5 pl-0.5 pr-2">{actions}</div>
        )}
      </div>
      {progress && (
        <ProgressBar
          value={progress.value}
          label={progress.label}
          className="h-0.5 rounded-none"
          indicatorClassName="rounded-none"
        />
      )}
      <Collapsible open={open}>
        <div
          id={panelId}
          role="region"
          aria-labelledby={triggerId}
          className={cn(
            // No body inset on a line shell: the material child owns its own, and a
            // second gutter here makes every result look nested twice.
            shell === "line" ? "pt-1.5 pb-1.5 pr-0" : "px-3 pb-2.5",
            contentClassName,
          )}
        >
          {children}
        </div>
      </Collapsible>
    </div>
  );
}
