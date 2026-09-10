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
  // Keep the state this fixture is about on screen once the list outgrows the window, and land
  // on the same pixel every time. Two things fight that: `scrollIntoView` answers in fractions,
  // and the browser clamps to a FRACTIONAL maximum — this list can scroll 18.6px — so reading
  // it back rounded to 18 on one run and 19 on the next, moving every row a pixel and blowing
  // past the goldens' tolerance. Integer offsets decide the target, flooring after the clamp
  // decides the landing, and the observer re-lands it when fonts finish and the height changes.
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!list) return;
    // From a fixed base every time, and this is the whole point rather than a tidy-up. Landing
    // only when the row is OUT of view makes the result depend on where the list already was:
    // the first land chose an offset against one set of row heights, the fonts arrived, the
    // observer re-landed — and found the row already in view, so it changed nothing and the
    // stale offset stayed. Two outcomes eleven pixels apart, roughly one run in six, which
    // reads as an unstable golden rather than as a landing that remembers.
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
    <div
      // The harness's own chrome, named so a sweep can tell it from the product. Measured
      // before this existed: on the agent route 23 of 51 buttons and 146 of 527 elements
      // belonged to this state switcher, and on the workspace route 28 of 52 and 198 of 773 —
      // so an audit that walked the page was reporting coverage of the test scaffold, and
      // would have reported a defect in it as a defect in the product.
      data-fixture-chrome=""
      {...stylex.props(fx.pane)}
    >
      {/* Empty, the way the product's drawer header is: the sidebar control is placed at
          the window-controls gutter and the header content box starts at the same edge, so
          anything written here is painted under the control. This caption is scaffolding
          and belongs with the scaffolding below it. */}
      <AgentSurfaceHeader corner="drawer" divider={false} />
      {/* The scrollport, because this list only grows: every view a round opens adds a row, and
          a state below the fold of a 720px window is a state whose golden cannot be taken. */}
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
      {/* The list takes the free height now; this is the gap above the caption, not a spring. */}
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
