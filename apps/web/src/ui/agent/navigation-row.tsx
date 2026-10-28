import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Icon, type IconName } from "@/ui/index";
import { reveal } from "@/ui/index";
import { Button, type ButtonProps } from "@flame/ui";
import { ItemHighlight, useHoverTrackItem } from "@flame/ui";
import { Tooltip } from "@flame/ui/tooltip";
import { color, leading, motion, space, surface, type as typeStep } from "@flame/ui/tokens.stylex";
import { AgentOverflowLabel } from "@/ui/agent/overflow-label";
const rowStyles = stylex.create({
  editor: { display: "flex", alignItems: "center", width: "100%" },
  base: {
    height: "auto",
    minHeight: "max(var(--density-row-height), var(--control-target-min))",
    gap: "var(--density-row-gap)",
    paddingInline: space.s2,
    borderWidth: 0,
    textAlign: "left",
    color: {
      default: color.fg,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": color.fg,
    },
    backgroundColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": surface.hover,
      ":focus-visible": surface.hover,
      ":is([data-active])": surface.selected,
      ":is([data-active]):is(:hover:not(:disabled, [aria-disabled=true], [data-disabled]), :focus-visible)":
        surface.selectedHover,
    },
    transitionProperty: "background-color, color",
    transitionDuration: motion.color,
    transitionTimingFunction: motion.easeState,
  },
  tracked: {
    backgroundColor: {
      default: "transparent",
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": "transparent",
      ":focus-visible": surface.hover,
      ":is([data-active])": surface.selected,
      ":is([data-active]):is(:hover:not(:disabled, [aria-disabled=true], [data-disabled]), :focus-visible)":
        surface.selected,
    },
    position: "relative",
    isolation: "isolate",
  },
  stacked: {
    height: "auto",
    alignItems: "flex-start",
    paddingBlock: space.s2,
  },
  nested: { paddingLeft: "var(--nested-row-indent)" },
  actioned: {
    paddingRight: {
      default: "calc(var(--spacing) * (2 + 6 * var(--reveal, 1)))",
      "@media (hover: none)": space.s8,
    },
  },
  fade: {
    transitionProperty: "opacity",
    transitionDuration: motion.fast,
    transitionTimingFunction: motion.easeState,
  },
  glyph: { flexShrink: 0, color: color.fg },
  glyphStacked: { marginTop: "1px" },
  stack: { display: "flex", minWidth: 0, flex: 1, flexDirection: "column", gap: "1px" },
  line: { display: "flex", minWidth: 0, alignItems: "center", gap: space.s2 },
  lineStacked: { lineHeight: leading.body },
  lineAlone: { lineHeight: leading.snug },
  label: { minWidth: 0, flex: 1 },
  trailing: { flexShrink: 0 },
  detail: { minWidth: 0, lineHeight: leading.body, color: color.fgFaint },
  host: { position: "relative", userSelect: "none" },
  actionSlot: {
    position: "absolute",
    insetBlock: 0,
    right: space.s1,
    display: "grid",
    placeItems: "center",
  },
  quiet: {
    color: {
      default: color.fgMuted,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": color.fg,
      ":is([data-active])": color.fg,
    },
  },
  search: {
    backgroundColor: {
      default: surface.hover,
      ":hover:not(:disabled, [aria-disabled=true], [data-disabled])": surface.selected,
    },
  },
});

const ROW_GROUP = [reveal.host] as const;

const RESTING_GLYPH = [reveal.displaced, rowStyles.fade] as const;

const HOVER_ACTION = [reveal.shown, rowStyles.fade] as const;

interface AgentRowProps extends Omit<ButtonProps, "children" | "variant" | "size" | "press"> {
  active?: boolean;
  icon?: IconName;
  detail?: ReactNode;
  trailing?: ReactNode;
  action?: ReactNode;
  indent?: "none" | "nested";
  look?: "row" | "quiet" | "search";
  revealOverflow?: boolean;
  children?: ReactNode;
}

export function AgentRow({
  active,
  icon,
  detail,
  trailing,
  action,
  indent = "none",
  look = "row",
  revealOverflow = false,
  className,
  styles: callerStyles,
  children,
  type = "button",
  ...props
}: AgentRowProps) {
  const trackItem = useHoverTrackItem();
  const track = look === "search" ? null : trackItem;
  const overflowText = revealOverflow && typeof children === "string" ? children : undefined;
  const label = stylex.props(rowStyles.label);
  const detailBox = stylex.props(rowStyles.detail, typeStep.uiXs);
  const button = (
    <Button
      data-chrome-focus={look === "search" ? undefined : ""}
      {...props}
      type={type}
      variant="ghost"
      size="sm"
      shape="row"
      active={active}
      onPointerEnter={(event) => {
        props.onPointerEnter?.(event);
        track?.onPointerEnter();
      }}
      styles={[
        rowStyles.base,
        typeStep.uiMd,
        detail ? rowStyles.stacked : null,
        indent === "nested" && rowStyles.nested,
        action ? rowStyles.actioned : null,
        look !== "row" && rowStyles.quiet,
        look === "search" && rowStyles.search,
        track ? rowStyles.tracked : null,
        callerStyles,
      ]}
      className={cn("agent-row", className)}
    >
      {track?.hovered && <ItemHighlight layoutId={track.layoutId} />}
      {icon && (
        <Icon
          name={icon}
          size="md"
          {...stylex.props(rowStyles.glyph, detail ? rowStyles.glyphStacked : null)}
        />
      )}
      <span {...stylex.props(rowStyles.stack)}>
        <span
          {...stylex.props(rowStyles.line, detail ? rowStyles.lineStacked : rowStyles.lineAlone)}
        >
          {overflowText ? (
            <AgentOverflowLabel text={overflowText} />
          ) : (
            <span {...label} className={cn(label.className, "truncate-fade")}>
              {children}
            </span>
          )}
          {trailing && (
            <span
              data-reveal={action ? "rest" : undefined}
              className={stylex.props(rowStyles.trailing, action ? RESTING_GLYPH : null).className}
            >
              {trailing}
            </span>
          )}
        </span>
        {detail != null && (
          <span {...detailBox} className={cn(detailBox.className, "truncate-fade")}>
            {detail}
          </span>
        )}
      </span>
    </Button>
  );
  const row = overflowText ? (
    <Tooltip label={overflowText} side="right" sideOffset={8} delayDuration={500}>
      {button}
    </Tooltip>
  ) : (
    button
  );

  if (!action) return row;
  return (
    <div className={stylex.props(rowStyles.host, ROW_GROUP).className}>
      {row}
      <span
        data-reveal="hover"
        className={stylex.props(rowStyles.actionSlot, HOVER_ACTION).className}
      >
        {action}
      </span>
    </div>
  );
}

export function AgentRowEditor({
  children,
  indent = "none",
}: {
  children: ReactNode;
  indent?: "none" | "nested";
}) {
  return (
    <div
      {...stylex.props(rowStyles.base, rowStyles.editor, indent === "nested" && rowStyles.nested)}
    >
      {children}
    </div>
  );
}
