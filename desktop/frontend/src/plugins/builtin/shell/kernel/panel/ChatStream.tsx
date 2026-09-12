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
import { vocab } from "@/ui";

const cst = stylex.create({
  /** A reading column with nothing in it takes no space, so the gap above it closes. */
  hideWhenEmpty: { display: { default: null, ":empty": "none" } },
  // The transcript answers its own width rather than the window's: a tool card decides whether
  // it can afford its meta column from THIS pane, which the dock resizes independently.
  queryable: { containerType: "inline-size" },
  // The rail hangs OUTSIDE the reading column, so it is positioned from the pane's centre plus
  // half the column — and only appears once the pane is wide enough to have room beside the
  // text. It spans the transcript and must not take the pointer from it; what it hangs there
  // takes the pointer back, in the one globals.css rule an atomic class cannot express.
  rail: {
    position: "absolute",
    top: 0,
    bottom: "var(--composer-overlay, 0px)",
    right: "calc(50% + var(--reading-column-max) / 2)",
    zIndex: 1,
    width: "var(--reading-rail-width)",
    flexDirection: "column",
    display: { default: "none", "@container (min-width: 1152px)": "flex" },
    pointerEvents: "none",
  },
  tray: {
    pointerEvents: "auto",
    display: "flex",
    flexDirection: "column",
    gap: space.s1_5,
    paddingBlock: space.s1_5,
  },
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

const ChatBanners = memo(function ChatBanners({ sessionId }: { sessionId: string }) {
  return (
    <div {...stylex.props(rc.box, rc.gutter, vocab.hold)}>
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
          <div
            {...readingBox}
            className={cn(stylex.props(cst.hideWhenEmpty).className, readingBox.className)}
          >
            <Slot name="chat.empty" />
          </div>
        </div>
      </>
    );
  }

  const pane = stylex.props(sh.paneAnchored);
  return (
    <div
      ref={paneRef}
      {...pane}
      className={cn(stylex.props(cst.queryable).className, pane.className)}
    >
      <ChatBanners sessionId={sessionId} />
      <div {...stylex.props(sh.paneAnchored)}>
        <div data-slot="chat-rail" {...stylex.props(cst.rail)}>
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
