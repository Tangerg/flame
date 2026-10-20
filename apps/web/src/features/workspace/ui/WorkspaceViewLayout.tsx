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
  children: ReactNode;
}

export function WorkspaceViewLayout({
  scrollInset = "rows",
  scrollRef,
  children,
  ...header
}: Props) {
  return (
    <AgentWorkspaceView>
      <ViewHeader {...header} />
      <ScrollArea
        ref={scrollRef}
        className={stylex.props(wl.viewport, scrollInset === "rows" && wl.rows).className}
      >
        {children}
      </ScrollArea>
    </AgentWorkspaceView>
  );
}
