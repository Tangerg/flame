import * as stylex from "@stylexjs/stylex";
import type { CSSProperties, ReactNode } from "react";
import { color, leading, layout, space, type, weight } from "./tokens.stylex";
import { Icon, type IconGlyph } from "./font-scale";
import { IconMark } from "./icon-mark";
import type { Tone } from "./tone";

export type EmptyStateSize = "compact" | "comfortable";

const styles = stylex.create({
  root: {
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    justifyContent: "center",
    textAlign: "center",
    color: color.fgFaint,
    minWidth: 0,
  },
  rootCompact: {
    gap: layout.labelGap,
    paddingInline: layout.compactGutter,
    paddingBlock: layout.sectionGap,
  },
  rootComfortable: {
    gap: space.s2_5,
    paddingInline: layout.cardPadding,
    paddingBlock: space.s12,
  },
  title: {
    fontWeight: weight.medium,
    letterSpacing: "normal",
    color: color.fg,
    overflowWrap: "anywhere",
    textWrap: "balance",
  },
  sub: {
    maxWidth: "min(100%,32ch)",
    lineHeight: leading.body,
    color: color.fgMuted,
    overflowWrap: "break-word",
  },
  action: { marginTop: layout.labelGap },
});

const ROOT = {
  compact: styles.rootCompact,
  comfortable: styles.rootComfortable,
} as const;
const TITLE_TYPE = { compact: type.uiXs, comfortable: type.uiMd } as const;

export function EmptyState({
  icon,
  title,
  sub,
  action,
  size = "comfortable",
  style,
  tone,
}: {
  icon?: IconGlyph;
  title: string;
  sub?: string;
  action?: ReactNode;
  size?: EmptyStateSize;
  style?: CSSProperties;
  tone?: Tone;
}) {
  return (
    <div {...stylex.props(styles.root, ROOT[size])} style={style}>
      {icon && (
        <IconMark size={size === "compact" ? "sm" : "lg"} tone={tone}>
          <Icon glyph={icon} size={size === "compact" ? "md" : "lg"} />
        </IconMark>
      )}
      <div {...stylex.props(TITLE_TYPE[size], styles.title)}>{title}</div>
      {sub && <div {...stylex.props(type.uiSm, styles.sub)}>{sub}</div>}
      {action && <div {...stylex.props(styles.action)}>{action}</div>}
    </div>
  );
}
