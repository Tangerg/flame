import * as stylex from "@stylexjs/stylex";
import { space } from "@/styles/tokens.stylex";
import type { ReactNode, Ref } from "react";
import { ScrollArea } from "@/ui";
import { AgentWorkspaceView } from "@/ui/agent";
import { ViewHeader, type ViewHeaderProps } from "./ViewHeader";

const wl = stylex.create({
  rows: { paddingBlock: space.s1 },
});

interface Props extends ViewHeaderProps {
  /**
   * How the scroller insets its rows. `rows` is the standing answer — fourteen of the
   * twenty-one views passed the same `py-1` to say it — and `flush` is for a view whose content
   * brings its own inset: a file, a terminal, a tree.
   */
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
        className={stylex.props(scrollInset === "rows" && wl.rows).className}
      >
        {children}
      </ScrollArea>
    </AgentWorkspaceView>
  );
}
