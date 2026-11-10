import * as stylex from "@stylexjs/stylex";
import { reveal } from "@/ui/atoms/reveal";
import { type ComponentPropsWithoutRef, type ReactNode } from "react";
import { Children, useId, useLayoutEffect, useRef } from "react";
import { cn } from "@/lib/classNames";
import { color, leading, motion, radius, space, surface, type } from "@flame/ui/tokens.stylex";
import { vocab } from "@flame/ui";
import { Collapsible, useDisclosedContent } from "@/ui/atoms/collapsible";
import { Pressable } from "@/ui/atoms/pressable";
import { ProgressBar } from "@flame/ui";
import { Icon, type IconName } from "@/ui/icons/index";
import { toneInk } from "@flame/ui/tone-ink";
import { chevron } from "@/ui/atoms/chevron";

const SCROLL_OVERFLOW = ["auto", "scroll", "hidden"];

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
  stuck: {
    position: { default: null, ":is([data-sticky])": "sticky" },
    top: { default: null, ":is([data-sticky])": 0 },
    zIndex: { default: null, ":is([data-sticky])": 1 },
  },
  stuckLine: { backgroundColor: { default: null, ":is([data-sticky])": surface.canvas } },
  stuckCard: { backgroundColor: { default: null, ":is([data-sticky])": surface.card } },
  trigger: {
    display: "flex",
    minWidth: 0,
    flex: 1,
    alignItems: "center",
    textAlign: "left",
    lineHeight: leading.body,
  },
  triggerFit: { flex: "0 1 auto" },
  triggerLine: {
    gap: space.s2,
    paddingBlock: space.s0_5,
    paddingRight: 0,
    paddingLeft: 0,
    minHeight: "max(var(--control-height-sm), var(--control-target-min))",
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
    color: "var(--row-ink)",
  },
  detail: {
    display: "flex",
    minWidth: 0,
    flex: 1,
    alignItems: "center",
    lineHeight: leading.snug,
    color: "var(--row-detail-ink)",
  },
  previewLine: { paddingInlineStart: space.s2 },
  previewCard: { paddingInlineEnd: space.s3 },
  spacer: { minWidth: 0, flex: 1 },
  trailing: {
    display: "flex",
    minWidth: 0,
    flexShrink: 1,
    alignItems: "center",
    gap: space.s1_5,
    color: color.fgFaint,
    fontVariantNumeric: "tabular-nums",
  },
  headerPublishes: {
    "--row-ink": { default: color.fgMuted, ":hover": color.fg },
    "--row-detail-ink": { default: color.fgFaint, ":hover": color.fgMuted },
  },
  headerPublishesDim: {
    "--row-ink": { default: color.fgFaint, ":hover": color.fgMuted },
    "--row-detail-ink": color.fgFaint,
  },
  chevron: {
    pointerEvents: "none",
    display: "flex",
    marginInlineStart: "calc(var(--spacing) * -1)",
    color: color.fgFaint,
  },
  actions: {
    display: "flex",
    flexShrink: 0,
    alignItems: "center",
    gap: space.s0_5,
    paddingLeft: space.s0_5,
    paddingRight: space.s2,
  },
  bodyLine: {
    marginInlineStart: space.s2,
    borderInlineStartWidth: "var(--control-edge-width)",
    borderInlineStartStyle: "solid",
    borderInlineStartColor: surface.field,
    paddingInlineStart: "calc(var(--spacing) * 4 - var(--control-edge-width))",
  },
  bodyCard: { paddingInline: space.s3 },
  bodyRows: { paddingBlock: space.s1_5 },
});

const TRAY_TONE = {
  neutral: styles.trayNeutral,
  warning: styles.trayWarning,
  negative: styles.trayNegative,
} as const;

const SHELL_SKINS = {
  line: {
    frame: styles.frameLine,
    sticky: styles.stuckLine,
    trigger: styles.triggerLine,
    mark: styles.markLine,
    text: type.uiSm,
    iconSize: "sm",
    preview: styles.previewLine,
    body: styles.bodyLine,
  },
  card: {
    frame: styles.frameCard,
    sticky: styles.stuckCard,
    trigger: styles.triggerCard,
    mark: styles.markCard,
    text: type.uiMd,
    iconSize: "md",
    preview: styles.previewCard,
    body: styles.bodyCard,
  },
} as const;

type AgentActivityDisclosureProps = Omit<ComponentPropsWithoutRef<"div">, "children"> &
  ActivityLeading & {
    label: ReactNode;
    detail?: ReactNode;
    // A glimpse of the shut body; it sits beside the toggle so changing text never renames the control.
    preview?: ReactNode;
    trailing?: ReactNode;
    actions?: ReactNode;
    feedback?: ReactNode;
    open: boolean;
    onToggle: () => void;
    stickyHeader?: boolean;
    progress?: { value: number; label: string };
    tone?: ActivityTone;
    ink?: "dim";
    shell: ActivityShell;
    summaryId?: string;
    children?: ReactNode;
    contentInset?: "rows";
    contentClassName?: string;
  };

export function AgentActivityDisclosure({
  icon,
  leading,
  label,
  detail,
  preview,
  trailing,
  actions,
  feedback,
  open,
  onToggle,
  stickyHeader,
  progress,
  tone = "neutral",
  ink,
  shell,
  summaryId,
  children,
  className,
  contentInset,
  contentClassName,
  ...props
}: AgentActivityDisclosureProps) {
  const skin = SHELL_SKINS[shell];
  const ownSummaryId = useId();
  const triggerId = summaryId ?? ownSummaryId;
  const panelId = useId();
  const framed = shell !== "line" && icon !== undefined;
  const disclosed = useDisclosedContent(open);
  const headerRef = useRef<HTMLDivElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const disclosable = children != null;

  useLayoutEffect(() => {
    const header = headerRef.current;
    const body = bodyRef.current;
    if (!stickyHeader || !header || !body) return;
    let viewport = header.parentElement?.parentElement ?? null;
    while (viewport && !SCROLL_OVERFLOW.includes(getComputedStyle(viewport).overflowY)) {
      viewport = viewport.parentElement;
    }
    if (!viewport) return;
    const scrollViewport = viewport;
    // Composer clearance resizes the scrolled content even when the viewport keeps its size.
    let content: Element = header;
    while (content.parentElement && content.parentElement !== scrollViewport) {
      content = content.parentElement;
    }
    const measure = () => {
      const height = header.getBoundingClientRect().height;
      const style = getComputedStyle(scrollViewport);
      const safeHeight =
        scrollViewport.getBoundingClientRect().height -
        (parseFloat(style.scrollPaddingTop) || 0) -
        (parseFloat(style.scrollPaddingBottom) || 0);
      const contentStyle = getComputedStyle(body.firstElementChild ?? body);
      const lineHeight = parseFloat(contentStyle.lineHeight);
      const inset =
        (parseFloat(contentStyle.marginTop) || 0) + (parseFloat(contentStyle.paddingTop) || 0);
      // Keep two body lines and their inset before reserving a summary above them.
      const fits = height > 0 && lineHeight > 0 && height + inset + 2 * lineHeight <= safeHeight;
      header.toggleAttribute("data-sticky", fits);
      body.toggleAttribute("data-activity-clearance", fits);
      if (fits) body.style.setProperty("--disclosure-header-height", `${height}px`);
      else body.style.removeProperty("--disclosure-header-height");
    };
    const view = header.ownerDocument.defaultView!;
    let frame = 0;
    // Publishing clearance resizes the observed content; deliver it outside that resize cycle.
    const observer = new ResizeObserver(() => {
      if (!frame)
        frame = view.requestAnimationFrame(() => {
          frame = 0;
          measure();
        });
    });
    observer.observe(header, { box: "border-box" });
    observer.observe(scrollViewport, { box: "border-box" });
    observer.observe(content, { box: "border-box" });
    measure();
    return () => {
      observer.disconnect();
      if (frame) view.cancelAnimationFrame(frame);
      header.removeAttribute("data-sticky");
      body.removeAttribute("data-activity-clearance");
      body.style.removeProperty("--disclosure-header-height");
    };
  }, [stickyHeader, disclosable]);

  return (
    <div
      {...props}
      data-slot="agent-activity-disclosure"
      data-tone={tone}
      data-shell={shell}
      className={cn(stylex.props(styles.frame, skin.frame).className, className)}
    >
      <div
        ref={headerRef}
        data-slot="agent-activity-header"
        className={cn(
          stylex.props(
            reveal.host,
            styles.header,
            ink === "dim" ? styles.headerPublishesDim : styles.headerPublishes,
            stickyHeader && [styles.stuck, skin.sticky],
          ).className,
        )}
      >
        <TriggerShell
          disclosable={disclosable}
          id={triggerId}
          panelId={panelId}
          open={open}
          onToggle={onToggle}
          className={
            stylex.props(styles.trigger, skin.trigger, preview != null && styles.triggerFit)
              .className
          }
        >
          <span
            aria-hidden
            data-slot="agent-activity-mark"
            data-framed={framed ? "" : undefined}
            data-tone={tone}
            {...stylex.props(
              styles.mark,
              skin.mark,
              framed && [styles.markFramed, TRAY_TONE[tone]],
              toneInk[tone],
            )}
          >
            {leading ?? (icon ? <Icon name={icon} size={skin.iconSize} /> : null)}
          </span>
          <span
            data-slot="agent-activity-label"
            {...stylex.props(styles.label, vocab.truncate, skin.text)}
          >
            {label}
          </span>
          {disclosable && (
            <span
              aria-hidden
              data-slot="agent-activity-chevron"
              data-open={open ? "" : undefined}
              {...stylex.props(chevron.base, styles.chevron, !open && chevron.shut)}
            >
              <Icon name="chevron-down" size="xs" />
            </span>
          )}
          {detail != null ? (
            <span {...stylex.props(styles.detail, vocab.truncate, skin.text)}>{detail}</span>
          ) : (
            preview == null && <span aria-hidden {...stylex.props(styles.spacer)} />
          )}
          {trailing != null && (
            <span {...stylex.props(styles.trailing, type.uiXs)}>{trailing}</span>
          )}
        </TriggerShell>
        {preview != null && (
          <span
            aria-hidden
            data-slot="agent-activity-preview"
            {...stylex.props(styles.detail, vocab.truncate, skin.text, skin.preview)}
          >
            {preview}
          </span>
        )}
        {Children.count(actions) > 0 && (
          <div data-focus-inset="" {...stylex.props(styles.actions)}>
            {actions}
          </div>
        )}
      </div>
      {progress && <ProgressBar value={progress.value} label={progress.label} weight="seam" />}
      {feedback}
      {disclosable && (
        <Collapsible open={open}>
          <div ref={bodyRef} {...stylex.props(skin.body)}>
            <div
              id={panelId}
              role="region"
              aria-labelledby={triggerId}
              className={cn(
                stylex.props(contentInset === "rows" && styles.bodyRows).className,
                contentClassName,
              )}
            >
              {disclosed && children}
            </div>
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
  className,
  children,
}: {
  disclosable: boolean;
  id: string;
  panelId: string;
  open: boolean;
  onToggle: () => void;
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
      data-focus-inset=""
      onClick={onToggle}
      className={className}
    >
      {children}
    </Pressable>
  );
}
