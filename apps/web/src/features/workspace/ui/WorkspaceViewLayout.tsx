import * as stylex from "@stylexjs/stylex";
import { space, surface } from "@flame/ui/tokens.stylex";
import { type ReactNode, type Ref } from "react";
import { ScrollArea } from "@/ui/index";
import { AgentWorkspaceView } from "@/ui/agent/index";
import { ViewHeader, type ViewHeaderProps } from "@/features/workspace/ui/ViewHeader";
const wl = stylex.create({
  viewport: { backgroundColor: surface.canvas },
  rows: { paddingBlock: space.s1 },
});

interface Props extends ViewHeaderProps {
  scrollInset?: "rows" | "flush";
  scrollRef?: Ref<HTMLDivElement>;
  // When the scrolled content has no focusable elements (a file viewer), name
  // the region so it is keyboard-focusable and scrollable.
  scrollLabel?: string;
  children: ReactNode;
}

export function WorkspaceViewLayout({
  scrollInset = "rows",
  scrollRef,
  scrollLabel,
  children,
  ...header
}: Props) {
  return (
    <AgentWorkspaceView>
      <ViewHeader {...header} />
      <ScrollArea
        ref={scrollRef}
        tabIndex={scrollLabel ? 0 : undefined}
        role={scrollLabel ? "region" : undefined}
        aria-label={scrollLabel}
        className={stylex.props(wl.viewport, scrollInset === "rows" && wl.rows).className}
      >
        {children}
      </ScrollArea>
    </AgentWorkspaceView>
  );
}
