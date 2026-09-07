import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { ScrollArea } from "@/ui/atoms/scroll-area";

const styles = stylex.create({
  // The gutter and the two section gaps are density tokens: the whole index breathes with the
  // UI size rather than each list picking a spacing.
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
  header: {
    display: "flex",
    alignItems: "center",
    gap: "calc(var(--spacing) * 1)",
    backgroundColor: "var(--app-drawer-surface)",
    paddingInline: "var(--density-navigation-gutter)",
    paddingBottom: "calc(var(--spacing) * 2.5)",
    paddingTop: "calc(var(--spacing) * 2)",
  },
});

export function AgentWorkIndexBody({ children }: { children: ReactNode }) {
  return (
    <ScrollArea
      hideScrollbar
      className={cn("agent-index-scroll", stylex.props(styles.scroll).className)}
    >
      <div {...stylex.props(styles.sections)}>{children}</div>
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
  return <div className={cn(stylex.props(styles.group).className, className)}>{children}</div>;
}

export function AgentWorkIndexFooter({ children }: { children: ReactNode }) {
  return <div {...stylex.props(styles.header)}>{children}</div>;
}
