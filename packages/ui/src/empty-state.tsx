import * as stylex from "@stylexjs/stylex";
import type { CSSProperties, ReactNode } from "react";
import { color, leading, layout, space, type, weight } from "./tokens.stylex";
import { Icon, type IconGlyph } from "./font-scale";
import { IconMark } from "./icon-mark";
import type { Tone } from "./tone";
import { toneInk } from "./tone-ink";
import { vocab } from "./vocabulary";

export type EmptyStateSize = "compact" | "comfortable";
export type EmptyStateAlign = "center" | "start";

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
  inline: {
    display: "flex",
    alignItems: "flex-start",
    gap: space.s2,
    paddingBlock: layout.rowPadding,
    textAlign: "start",
    color: color.fgFaint,
    minWidth: 0,
  },
  // Stands in for a compact list row, so it keeps that row's inset.
  inlineCompact: { paddingInline: space.s2, paddingBlock: space.s1_5 },
  inlineCopy: {
    display: "flex",
    flexDirection: "column",
    alignItems: "flex-start",
    gap: layout.labelGap,
    flex: 1,
    minWidth: 0,
  },
  title: {
    fontWeight: weight.medium,
    letterSpacing: "normal",
    color: color.fg,
    overflowWrap: "anywhere",
    textWrap: "balance",
  },
  sub: {
    lineHeight: leading.body,
    color: color.fgMuted,
    overflowWrap: "break-word",
    textWrap: "pretty",
  },
  centeredSub: { maxWidth: "min(100%,32ch)" },
  action: { marginTop: layout.labelGap },
});

const ROOT = {
  compact: styles.rootCompact,
  comfortable: styles.rootComfortable,
} as const;
const TITLE_TYPE = { compact: type.uiXs, comfortable: type.uiMd } as const;
// A note never reads larger than the title it explains.
const SUB_TYPE = { compact: type.uiXs, comfortable: type.uiSm } as const;
const GLYPH_SIZE = { compact: "xs", comfortable: "sm" } as const;

export function EmptyState({
  icon,
  title,
  sub,
  action,
  size = "comfortable",
  align = "center",
  style,
  tone,
}: {
  icon?: IconGlyph;
  title: string;
  sub?: string;
  action?: ReactNode;
  size?: EmptyStateSize;
  align?: EmptyStateAlign;
  style?: CSSProperties;
  tone?: Tone;
}) {
  const centered = align === "center";
  const copy = (
    <>
      {/* Preserve word boundaries when a containing control includes this feedback in its name. */}
      <div {...stylex.props(TITLE_TYPE[size], styles.title)}>{title} </div>
      {sub && (
        <div
          {...stylex.props(
            SUB_TYPE[size],
            styles.sub,
            centered && styles.centeredSub,
          )}
        >
          {sub}{" "}
        </div>
      )}
      {action && <div {...stylex.props(styles.action)}>{action}</div>}
    </>
  );
  if (!centered) {
    return (
      <div
        {...stylex.props(
          styles.inline,
          size === "compact" && styles.inlineCompact,
        )}
        style={style}
      >
        {icon && (
          // Shares the title's type step so its 1lh box centres the glyph on the title line.
          <span
            {...stylex.props(
              TITLE_TYPE[size],
              vocab.firstLine,
              vocab.hold,
              tone && toneInk[tone],
            )}
          >
            <Icon glyph={icon} size={GLYPH_SIZE[size]} />
          </span>
        )}
        <div {...stylex.props(styles.inlineCopy)}>{copy}</div>
      </div>
    );
  }
  return (
    <div {...stylex.props(styles.root, ROOT[size])} style={style}>
      {icon && (
        <IconMark size={size === "compact" ? "sm" : "lg"} tone={tone}>
          <Icon glyph={icon} size={size === "compact" ? "md" : "lg"} />
        </IconMark>
      )}
      {copy}
    </div>
  );
}
