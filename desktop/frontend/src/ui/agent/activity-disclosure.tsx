import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { Children, useId } from "react";
import { cn } from "@/lib/classNames";
import { Collapsible } from "@/ui/atoms/collapsible";
import { Pressable } from "@/ui/atoms/pressable";
import { ProgressBar } from "@/ui/atoms/progress-bar";
import { Icon, type IconName } from "@/ui/icons";

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
    shell: ActivityShell;
    children: ReactNode;
    contentClassName?: string;
  };

const TONE_CLASS: Record<ActivityTone, string> = {
  neutral: "text-fg-muted",
  warning: "text-warning",
  negative: "text-negative",
};

const TRAY_CLASS: Record<ActivityTone, string> = {
  neutral: "bg-surface-2",
  warning: "bg-warning-badge",
  negative: "bg-negative-badge",
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
        "min-w-0 overflow-clip",
        shell === "line"
          ? "rounded-[var(--shape-sm)]"
          : "rounded-[var(--surface-card-radius)] bg-card",
        className,
      )}
    >
      <div
        className={cn(
          "group/activity-header flex min-w-0 items-center",
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
            "group/activity-trigger flex min-w-0 flex-1 items-center text-left",
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
              // Identity, not decoration: one glyph per tool is the fastest read on the row,
              // and the faintest tone spends that distinction on nothing.
              shell === "line" && tone === "neutral" ? "text-fg-muted" : TONE_CLASS[tone],
            )}
          >
            {leading ?? (icon ? <Icon name={icon} size="xs" /> : null)}
          </span>
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
            <span className="flex shrink-0 items-center gap-1.5 font-mono text-ui-2xs text-fg-faint">
              {trailing}
            </span>
          )}
          <span
            aria-hidden
            data-slot="agent-activity-chevron"
            data-reveal="hover"
            className={cn(
              "flex shrink-0 text-fg-faint transition-[transform,opacity] duration-[var(--dur-fast)]",
              // Keyed on the TRIGGER's own `:focus-visible`, not on the header's
              // `:focus-within`: DOM focus outlives the pointer, so a row clicked shut
              // kept its chevron lit while every identical row beside it stayed blank.
              // `:has(:focus-visible)` would say the same thing and does not work —
              // Chromium matches the selector but never invalidates the subtree when
              // focus-visible changes inside `:has()`, so the reveal only lands if some
              // unrelated recalculation happens to follow.
              "group-focus-visible/activity-trigger:opacity-100 group-hover/activity-header:opacity-100",
              open ? "opacity-100" : "-rotate-90 opacity-0",
            )}
          >
            <Icon name="chevron-down" size="xs" />
          </span>
        </Pressable>
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
          className={cn(shell === "line" ? "pt-1.5 pb-1.5 pr-0" : "px-3 pb-2.5", contentClassName)}
        >
          {children}
        </div>
      </Collapsible>
    </div>
  );
}
