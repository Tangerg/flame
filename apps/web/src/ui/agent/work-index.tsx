import * as stylex from "@stylexjs/stylex";
import { type CSSProperties, type ReactNode } from "react";
import { clsx } from "clsx";
import { HoverTrack } from "@flame/ui";
import { ScrollArea } from "@/ui/atoms/scroll-area";
import { space } from "@flame/ui/tokens.stylex";
const styles = stylex.create({
  scroll: {
    paddingInline: "var(--density-navigation-gutter)",
    paddingBottom: "calc(var(--spacing) * 5)",
    paddingTop: "calc(var(--spacing) * 2)",
  },
  sections: {
    display: "flex",
    flexDirection: "column",
    rowGap: "var(--density-navigation-section-gap)",
  },
  group: { display: "flex", flexDirection: "column", gap: "var(--density-navigation-group-gap)" },
  item: { minWidth: 0 },
  footer: {
    display: "flex",
    alignItems: "center",
    gap: "calc(var(--spacing) * 1)",
    backgroundColor: "var(--app-drawer-surface)",
    paddingInline: "var(--density-navigation-gutter)",
    paddingBottom: "calc(var(--spacing) * 2.5)",
    paddingTop: "calc(var(--spacing) * 2)",
  },
});

// An empty section stands in for a compact row, so it keeps that row's inset.
export const AGENT_WORK_INDEX_EMPTY_INSET: CSSProperties = {
  paddingInline: space.s2,
  paddingBlock: space.s1_5,
};

export function AgentWorkIndexBody({ children }: { children: ReactNode }) {
  return (
    <ScrollArea className={clsx("agent-index-scroll", stylex.props(styles.scroll).className)}>
      <HoverTrack styles={styles.sections}>{children}</HoverTrack>
    </ScrollArea>
  );
}

export function AgentWorkIndexSection({ children }: { children: ReactNode }) {
  return <div {...stylex.props(styles.item)}>{children}</div>;
}

export function AgentWorkIndexGroupList({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <div className={clsx(stylex.props(styles.group).className, className)}>{children}</div>;
}

export function AgentWorkIndexFooter({ children }: { children: ReactNode }) {
  return <div {...stylex.props(styles.footer)}>{children}</div>;
}
