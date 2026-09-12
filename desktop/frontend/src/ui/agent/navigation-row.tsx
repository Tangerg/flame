import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { Button, Icon, reveal, type ButtonProps, type IconName } from "@/ui";
import { Tooltip } from "@/ui/atoms/tooltip";
import { color, leading, motion, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { AgentOverflowLabel } from "./overflow-label";

// The agent row's own shape, composed INTO the button rather than layered over it: every one of
// these replaces a property the button declared, which a class list beside it could not do.
const rowStyles = stylex.create({
  base: {
    height: "var(--density-row-height)",
    gap: "var(--density-row-gap)",
    paddingInline: space.s2,
    textAlign: "left",
    color: { default: color.fg, ":hover": color.fg },
    backgroundColor: {
      default: "transparent",
      ":hover": surface.hover,
      ":focus-visible": surface.hover,
      ":is([data-active])": surface.selected,
      ":is([data-active]):is(:hover, :focus-visible)": surface.selectedHover,
    },
    transitionProperty: "background-color, color",
    transitionDuration: "var(--dur-color)",
  },
  stacked: {
    height: "auto",
    minHeight: "var(--density-row-height)",
    alignItems: "flex-start",
    paddingBlock: space.s2,
  },
  nested: { paddingLeft: "calc(0.5rem + var(--icon-sm) + var(--density-row-gap))" },
  actioned: { paddingRight: space.s8 },
  // The fade is the element's own, not the reveal channel's: a transition-property declaration
  // is the whole list, so whatever also moves has to be named beside it.
  fade: { transitionProperty: "opacity", transitionDuration: motion.fast },
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
    color: { default: color.fgMuted, ":hover": color.fg, ":is([data-active])": color.fg },
  },
  search: { backgroundColor: { default: surface.sunken, ":hover": surface.sunkenHover } },
});

// Styles rather than class strings, so each one composes INTO the props call at its element
// instead of being concatenated beside it: the pair below has to stay one decision.
const ROW_GROUP = [reveal.host] as const;

// The SAME conditions as `HOVER_ACTION` below, and it has to be the same: one is what the
// other displaces, so a state that reveals the action without retiring this leaves the row
// showing both.
const RESTING_GLYPH = [reveal.displaced, rowStyles.fade] as const;

// The action is the caller's node in a sibling span, so only the SPAN can react to it
// having focus — and `:has(:focus-visible)` is not a working way to say that (Chromium
// matches it but does not invalidate on the focus change). `:focus-within` therefore
// stays here: it reveals the action a moment longer than it should after a click, which
// is the lesser of the two, because the alternative hides it from the keyboard.
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
  // Destructured rather than spread: the array below is passed AFTER `{...props}`, so a
  // caller's `styles` would be dropped on the floor.
  styles: callerStyles,
  children,
  type = "button",
  ...props
}: AgentRowProps) {
  const overflowText = revealOverflow && typeof children === "string" ? children : undefined;
  // `truncate-fade` is the mask, and it has to SURVIVE: spreading `stylex.props` after a
  // `className` replaces it, which drops the clip and lets the label push the row wide.
  const label = stylex.props(rowStyles.label);
  const detailBox = stylex.props(rowStyles.detail, typeStep.ui2xs);
  const button = (
    <Button
      {...props}
      type={type}
      variant="ghost"
      size="sm"
      shape="row"
      active={active}
      styles={[
        rowStyles.base,
        typeStep.uiMd,
        detail ? rowStyles.stacked : null,
        indent === "nested" && rowStyles.nested,
        action ? rowStyles.actioned : null,
        look !== "row" && rowStyles.quiet,
        look === "search" && rowStyles.search,
        callerStyles,
      ]}
      className={cn("agent-row", className)}
    >
      {icon && (
        <Icon
          name={icon}
          size="sm"
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
