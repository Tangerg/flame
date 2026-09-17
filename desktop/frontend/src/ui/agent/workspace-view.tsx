import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";

const styles = stylex.create({
  view: { display: "flex", minHeight: 0, flex: 1, flexDirection: "column" },
  row: { display: "flex", minHeight: 0, flex: 1 },
  main: { display: "flex", minHeight: 0, minWidth: 0, flex: 1, flexDirection: "column" },
});

export function AgentWorkspaceView({
  children,
  className,
  ariaLabel,
}: {
  children: ReactNode;
  className?: string;
  ariaLabel?: string;
}) {
  return (
    <div
      role={ariaLabel === undefined ? undefined : "region"}
      aria-label={ariaLabel}
      className={cn("agent-workspace-view", stylex.props(styles.view).className, className)}
    >
      {children}
    </div>
  );
}
