import { useLayoutEffect, useRef } from "react";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import { selectCurrentRootAttention } from "@/plugins/builtin/agent/application/view/runTree";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { useSendComposerInput } from "@/plugins/builtin/chat/composer/public/sendToAgent";
import { ChatPanel } from "@/plugins/builtin/shell/kernel/panel/ChatPanel";
import { AgentAppShell, AgentRow, AgentSurfaceHeader } from "@/ui/agent";
import type { VisualAgentState } from "./agentSessionSnapshots";
import * as stylex from "@stylexjs/stylex";
import { fx } from "./fixtureStyles";
import { type as typeStep } from "@/styles/tokens.stylex";

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
  "error-retryable": "Error, recoverable",
  recovery: "Recovery",
  "cwd-missing": "Folder gone",
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
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const land = () => {
      const active = list.querySelector<HTMLElement>("[data-active]");
      if (!active) return;
      list.scrollTop = 0;
      const bottom = active.offsetTop + active.offsetHeight;
      if (bottom > list.clientHeight) list.scrollTop = Math.floor(bottom - list.clientHeight);
    };
    land();
    const observer = new ResizeObserver(land);
    observer.observe(list);
    return () => observer.disconnect();
  }, [state]);
  return (
    <div data-fixture-chrome="" {...stylex.props(fx.pane)}>
      <AgentSurfaceHeader corner="drawer" divider={false} />
      <div ref={listRef} {...stylex.props(fx.scroller)}>
        <span {...stylex.props(fx.listHead, typeStep.uiMd)}>Agent states</span>
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
      <div {...stylex.props(fx.footGap)} />
      <div {...stylex.props(fx.listFoot, typeStep.uiXs)}>
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
          {...stylex.props(fx.contents)}
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
