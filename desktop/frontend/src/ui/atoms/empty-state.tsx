import * as stylex from "@stylexjs/stylex";
import type { CSSProperties, ReactNode } from "react";
import { color, corner, leading, space, surface, type } from "@/styles/tokens.stylex";
import { Icon, type IconName } from "@/ui/icons";

/** How much room the state is given. `data-view` names the same two. */
export type EmptyStateSize = "compact" | "comfortable";

const styles = stylex.create({
  root: {
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    justifyContent: "center",
    textAlign: "center",
    color: color.fgFaint,
    userSelect: "none",
  },
  rootCompact: { gap: space.s1_5, paddingInline: space.s4, paddingBlock: space.s6 },
  rootComfortable: { gap: space.s2_5, paddingInline: space.s5, paddingBlock: space.s12 },
  icon: {
    display: "grid",
    placeItems: "center",
    backgroundColor: surface.surface2,
    color: color.fgMuted,
  },
  iconCompact: { height: space.s7, width: space.s7 },
  iconComfortable: { height: space.s10, width: space.s10 },
  // The one line the eye lands on first, so it opts out of the UI tracking the step carries:
  // a heading read alone does not need the crowding that keeps a dense row legible.
  title: { fontWeight: 500, letterSpacing: "normal", color: color.fg },
  sub: { maxWidth: "280px", lineHeight: leading.body, color: color.fgMuted },
  action: { marginTop: space.s1_5 },
});

const ROOT = { compact: styles.rootCompact, comfortable: styles.rootComfortable } as const;
const ICON = { compact: styles.iconCompact, comfortable: styles.iconComfortable } as const;
const TITLE_TYPE = { compact: type.uiXs, comfortable: type.uiMd } as const;

export function EmptyState({
  icon,
  title,
  sub,
  action,
  size = "comfortable",
  style,
}: {
  icon?: IconName;
  title: string;
  sub?: string;
  action?: ReactNode;
  size?: EmptyStateSize;
  style?: CSSProperties;
}) {
  return (
    <div {...stylex.props(styles.root, ROOT[size])} style={style}>
      {icon && (
        <div {...stylex.props(styles.icon, corner.pill, ICON[size])}>
          <Icon name={icon} size={size === "compact" ? "md" : "lg"} />
        </div>
      )}
      <div {...stylex.props(TITLE_TYPE[size], styles.title)}>{title}</div>
      {sub && <div {...stylex.props(type.uiSm, styles.sub)}>{sub}</div>}
      {action && <div {...stylex.props(styles.action)}>{action}</div>}
    </div>
  );
}
