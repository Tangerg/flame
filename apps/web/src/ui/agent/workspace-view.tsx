import * as stylex from "@stylexjs/stylex";
import { type ReactNode, type Ref } from "react";
import { cn } from "@/lib/classNames";
const styles = stylex.create({
  view: { display: "flex", minHeight: 0, flex: 1, flexDirection: "column" },
});

export function AgentWorkspaceView({
  children,
  className,
  ariaLabel,
  ref,
  tabIndex,
}: {
  children: ReactNode;
  className?: string;
  ariaLabel?: string;
  ref?: Ref<HTMLDivElement>;
  tabIndex?: number;
}) {
  return (
    <div
      ref={ref}
      tabIndex={tabIndex}
      role={ariaLabel === undefined ? undefined : "region"}
      aria-label={ariaLabel}
      className={cn("agent-workspace-view", stylex.props(styles.view).className, className)}
    >
      {children}
    </div>
  );
}
