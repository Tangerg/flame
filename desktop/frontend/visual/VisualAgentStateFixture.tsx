import { useLayoutEffect, useRef } from "react";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import { selectCurrentRootAttention } from "@/plugins/builtin/agent/application/view/runTree";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { useSendComposerInput } from "@/plugins/builtin/chat/composer/public/sendToAgent";
import { ChatPanel } from "@/plugins/builtin/shell/kernel/panel/ChatPanel";
import { AgentAppShell, AgentRow, AgentSurfaceHeader } from "@/ui/agent";
import type { VisualAgentState } from "./agentSessionSnapshots";

const STATE_LABELS: Record<VisualAgentState, string> = {
  empty: "Empty",
  idle: "Idle",
  running: "Running",
  "answer-opening": "Answer opening",
  steer: "Steer",
  waiting: "Waiting",
  question: "Question",
  terminal: "Terminal",
  canceled: "Canceled",
  error: "Error",
  recovery: "Recovery",
  delegated: "Delegated tree",
  "long-content": "Long content",
  narrative: "Narrative",
  "tool-shells": "Tool shells",
  "tool-search": "Tool search",
  "tool-remote": "Tool remote",
  "tool-agentic": "Tool agentic",
  "tool-tail": "Tool tail",
  "question-multi": "Question · multi",
  waves: "Waves",
};

function StateSidebar({ state }: { state: VisualAgentState }) {
  const listRef = useRef<HTMLDivElement>(null);
  // Keep the state this fixture is about on screen once the list outgrows the window, and land
  // on the same pixel every time. Two things fight that: `scrollIntoView` answers in fractions,
  // and the browser clamps to a FRACTIONAL maximum — this list can scroll 18.6px — so reading
  // it back rounded to 18 on one run and 19 on the next, moving every row a pixel and blowing
  // past the goldens' tolerance. Integer offsets decide the target, flooring after the clamp
  // decides the landing, and the observer re-lands it when fonts finish and the height changes.
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const land = () => {
      const active = list.querySelector<HTMLElement>("[data-active]");
      if (!active) return;
      const top = active.offsetTop;
      const bottom = top + active.offsetHeight;
      if (top < list.scrollTop) list.scrollTop = top;
      else if (bottom > list.scrollTop + list.clientHeight) {
        list.scrollTop = bottom - list.clientHeight;
      }
      list.scrollTop = Math.floor(list.scrollTop);
    };
    land();
    const observer = new ResizeObserver(land);
    observer.observe(list);
    return () => observer.disconnect();
  }, [state]);
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Empty, the way the product's drawer header is: the sidebar control is placed at
          the window-controls gutter and the header content box starts at the same edge, so
          anything written here is painted under the control. This caption is scaffolding
          and belongs with the scaffolding below it. */}
      <AgentSurfaceHeader corner="drawer" divider={false} />
      {/* The scrollport, because this list only grows: every view a round opens adds a row, and
          a state below the fold of a 720px window is a state whose golden cannot be taken. */}
      <div ref={listRef} className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-2 pt-2">
        <span className="px-2 pb-1 text-ui-md font-semibold text-fg">Agent states</span>
        {(Object.keys(STATE_LABELS) as VisualAgentState[]).map((candidate) => (
          <AgentRow
            key={candidate}
            icon={candidate === "error" ? "alert" : candidate === "delegated" ? "bot" : "chat"}
            active={candidate === state}
          >
            {STATE_LABELS[candidate]}
          </AgentRow>
        ))}
      </div>
      {/* The list takes the free height now; this is the gap above the caption, not a spring. */}
      <div className="min-h-4" />
      <div className="px-4 pb-3 text-ui-xs leading-body text-fg-faint">
        Canonical snapshot → production projection
      </div>
    </div>
  );
}

export function VisualAgentStateFixture({
  state,
  view,
}: {
  state: VisualAgentState;
  view: AgentSessionView;
}) {
  const attention = selectCurrentRootAttention(view);
  // Exercise the same Composer → agent input bridge as the production kernel;
  // the visual agent port records the payload for interaction assertions.
  const send = useSendComposerInput();

  return (
    <AgentAppShell
      sidebarLabel="Agent fixture states"
      sidebarResizeLabel="Resize the agent fixture sidebar"
      sidebarOpen
      sidebarWidth={SIDEBAR_DEFAULT_WIDTH_PX}
      onResize={() => undefined}
      onSidebarToggle={() => undefined}
      sidebarExpandLabel="Expand the agent fixture sidebar"
      sidebarCollapseLabel="Collapse the agent fixture sidebar"
      sidebar={<StateSidebar state={state} />}
      main={
        <div
          className="contents"
          data-testid="agent-state"
          data-state={state}
          data-attention={attention.status}
        >
          <ChatPanel onSend={send} />
        </div>
      }
    />
  );
}
