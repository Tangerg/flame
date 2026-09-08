import * as stylex from "@stylexjs/stylex";
import { cn } from "@/lib/classNames";
import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import { memo, useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { useActiveConversationRows } from "@/plugins/builtin/agent/public/conversation";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { Slot } from "@/plugins/host/Slot";
import {
  reconcileWorkspaceToolSelection,
  useExpandedWorkspaceToolIds,
  useSelectWorkspaceTool,
  useToggleWorkspaceTool,
} from "@/plugins/builtin/workspace/public/navigation";
import { useStreamRevealStore } from "@/plugins/builtin/chat/message/public/streamReveal";
import { ChatErrorBoundary } from "./ChatErrorBoundary";
import { ComposerSurface } from "./ComposerSurface";
import { ComposerOverlayTop, FloatingComposer, RuntimeConnectionNotice } from "./FloatingComposer";
import { COMPOSER_OVERLAY_PROPERTY, readingColumn as rc } from "./readingColumn";
import { CwdMissingBanner } from "./CwdMissingBanner";
import { MessageStream, type MessageStreamController } from "./MessageStream";
import { RunErrorBanner } from "./RunErrorBanner";
import { EmptyChatHeading } from "./ProjectSelector";
import {
  pendingQuestionRequest,
  QuestionCard,
} from "@/plugins/builtin/chat/message/public/rendering";
import { shellStyles as sh } from "../shellStyles";
import { color, space, type as typeStep, weight } from "@/styles/tokens.stylex";

const cst = stylex.create({
  tray: {
    pointerEvents: "auto",
    display: "flex",
    flexDirection: "column",
    gap: space.s1_5,
    paddingBlock: space.s1_5,
  },
  // The empty transcript centres its greeting and keeps a sixth of the pane clear beneath it,
  // so the composer below does not read as the bottom of a full page.
  empty: {
    display: "flex",
    minHeight: 0,
    flex: 1,
    flexDirection: "column",
    alignItems: "center",
    justifyContent: "center",
    gap: space.s5,
    overflowY: "auto",
    overscrollBehavior: "contain",
    paddingBottom: "6vh",
  },
  heading: {
    marginInline: "auto",
    maxWidth: "620px",
    textWrap: "balance",
    textAlign: "center",
    color: color.fg,
    fontWeight: weight.medium,
  },
});

interface Props {
  onSend: (input: AgentInput) => boolean;
}

// The rail hangs OUTSIDE the reading column, so it is positioned from the window's centre
// plus half the column — and only appears once the window is wide enough to have room beside
// the text. `[&>*]` stays a utility: the rail is transparent to the pointer and its children
// are not, which is a descendant rule no atomic class can express.
const RAIL =
  "absolute top-0 bottom-[var(--composer-overlay,0px)] z-1 hidden w-[var(--reading-rail-width)] flex-col @min-[1152px]:flex pointer-events-none [&>*]:pointer-events-auto right-[calc(50%+var(--reading-column-max)/2)]";

const ChatBanners = memo(function ChatBanners({ sessionId }: { sessionId: string }) {
  return (
    <div {...stylex.props(rc.box, rc.gutter, sh.hold)}>
      <CwdMissingBanner key={sessionId} />
      <RunErrorBanner />
      <Slot name="chat.banner.top" wrapper {...stylex.props(cst.tray)} />
    </div>
  );
});

export function ChatStream({ onSend }: Props) {
  const sessionId = useActiveSessionId();
  const rows = useActiveConversationRows();
  const toolCalls = useActiveSessionToolCalls();

  const expandedToolIds = useExpandedWorkspaceToolIds();
  const selectTool = useSelectWorkspaceTool();
  const toggleExpandedTool = useToggleWorkspaceTool();

  const textReveal = useStreamRevealStore((state) => state.streamReveal);

  const toolIdSignature = useMemo(() => Object.keys(toolCalls).join("\u001f"), [toolCalls]);
  const toolIds = useMemo(
    () => (toolIdSignature ? toolIdSignature.split("\u001f") : []),
    [toolIdSignature],
  );
  useEffect(() => {
    reconcileWorkspaceToolSelection(toolIds);
  }, [toolIds]);

  const ctx = useMemo(
    () => ({
      onSelectTool: selectTool,
      expandedIds: expandedToolIds,
      onToggleExpand: toggleExpandedTool,
      textReveal,
    }),
    [selectTool, expandedToolIds, toggleExpandedTool, textReveal],
  );

  const pendingQuestion = useMemo(() => pendingQuestionRequest(rows), [rows]);
  const composer = pendingQuestion ? (
    <QuestionCard {...pendingQuestion} />
  ) : (
    <ComposerSurface onSend={onSend} />
  );
  const started = rows.length > 0;

  const paneRef = useRef<HTMLDivElement>(null);
  const composerOverlayRef = useRef<HTMLDivElement>(null);
  const messageStreamRef = useRef<MessageStreamController>(null);

  useLayoutEffect(() => {
    if (!started) return;
    const pane = paneRef.current;
    const overlay = composerOverlayRef.current;
    if (!pane || !overlay) return;

    const publishHeight = (height = overlay.getBoundingClientRect().height) => {
      pane.style.setProperty(COMPOSER_OVERLAY_PROPERTY, `${height}px`);
    };

    publishHeight();
    messageStreamRef.current?.settleInitialBottom();
    const observer = new ResizeObserver(([entry]) => {
      const borderBox = entry?.borderBoxSize[0];
      publishHeight(borderBox?.blockSize);
    });
    observer.observe(overlay);
    return () => {
      observer.disconnect();
    };
  }, [started]);

  if (!started) {
    const emptyPane = stylex.props(cst.empty);
    const readingBox = stylex.props(rc.box, rc.gutter);
    return (
      <>
        <ChatBanners sessionId={sessionId} />
        <div {...emptyPane} className={cn("panel-scroll", emptyPane.className)}>
          <div {...stylex.props(rc.box, rc.gutter)}>
            <h2 {...stylex.props(cst.heading, typeStep.displayMd)}>
              <EmptyChatHeading />
            </h2>
          </div>
          <div {...stylex.props(rc.box, rc.gutter)}>
            <ComposerOverlayTop />
            <RuntimeConnectionNotice />
            {composer}
          </div>
          <div {...readingBox} className={cn("empty:hidden", readingBox.className)}>
            <Slot name="chat.empty" />
          </div>
        </div>
      </>
    );
  }

  const pane = stylex.props(sh.paneAnchored);
  return (
    <div ref={paneRef} {...pane} className={cn("@container", pane.className)}>
      <ChatBanners sessionId={sessionId} />
      <div {...stylex.props(sh.paneAnchored)}>
        <div className={RAIL}>
          <Slot name="chat.rail.start" />
        </div>
        <div {...stylex.props(sh.paneAnchored)}>
          <ChatErrorBoundary resetKey={sessionId} label={`session:${sessionId}`}>
            <MessageStream
              rows={rows}
              ctx={ctx}
              sessionId={sessionId}
              controllerRef={messageStreamRef}
            />
          </ChatErrorBoundary>
        </div>

        <FloatingComposer overlayRef={composerOverlayRef}>{composer}</FloatingComposer>
      </div>
    </div>
  );
}
