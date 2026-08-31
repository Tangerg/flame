import { useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { EmptyState } from "@/ui";
import { useT } from "@/lib/i18n";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { workspaceCommandActivitiesFromAgentTools } from "../application/toolActivity";
import { TerminalViewModel, terminalSubtext } from "../application/terminalViewModel";
import { CommandLog } from "./views/CommandLog";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useSelectedWorkspaceToolId } from "@/plugins/builtin/workspace/public/navigation";

export function TerminalWorkspaceSurface() {
  const t = useT();
  const toolCalls = useActiveSessionToolCalls();
  const selectedToolId = useSelectedWorkspaceToolId();
  const view = useMemo(
    () => TerminalViewModel.from(workspaceCommandActivitiesFromAgentTools(toolCalls)),
    [toolCalls],
  );
  const selectedCommandId = view.selectedCommandId(selectedToolId);

  const scrollRef = useRef<HTMLDivElement>(null);
  const pinnedRef = useRef(true);
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onScroll = () => {
      pinnedRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);
  useLayoutEffect(() => {
    if (!selectedCommandId) {
      pinnedRef.current = true;
      return;
    }
    pinnedRef.current = view.latestCommandId === selectedCommandId;
    scrollRef.current
      ?.querySelector<HTMLElement>("[data-command-selected]")
      ?.scrollIntoView?.({ block: "nearest" });
  }, [selectedCommandId, view.latestCommandId]);
  useEffect(() => {
    if (!pinnedRef.current) return;
    const el = scrollRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [view]);

  return (
    <WorkspaceViewLayout
      icon="terminal"
      title="terminal.title"
      sub={terminalSubtext(t, view)}
      scrollRef={scrollRef}
    >
      {view.isEmpty ? (
        <EmptyState
          icon="terminal"
          title={t("terminal.empty.title")}
          sub={t("terminal.empty.sub")}
        />
      ) : (
        <CommandLog commands={view.commands} selectedCommandId={selectedCommandId} />
      )}
    </WorkspaceViewLayout>
  );
}
