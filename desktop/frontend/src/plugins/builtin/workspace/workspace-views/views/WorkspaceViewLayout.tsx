import type { ReactNode, Ref } from "react";
import { ScrollArea } from "@/ui";
import { AgentWorkspaceView } from "@/ui/agent";
import { ViewHeader, type ViewHeaderProps } from "./ViewHeader";

interface Props extends ViewHeaderProps {
  scrollClassName?: string;
  scrollRef?: Ref<HTMLDivElement>;
  children: ReactNode;
}

export function WorkspaceViewLayout({ scrollClassName, scrollRef, children, ...header }: Props) {
  return (
    <AgentWorkspaceView>
      <ViewHeader {...header} />
      <ScrollArea ref={scrollRef} className={scrollClassName}>
        {children}
      </ScrollArea>
    </AgentWorkspaceView>
  );
}
