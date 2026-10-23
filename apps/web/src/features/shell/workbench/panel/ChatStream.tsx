import * as stylex from "@stylexjs/stylex";
import { tupleKey } from "@/lib/tupleKey";
import { type AgentInput } from "@/features/agent/public/input";
import { memo, useLayoutEffect, useMemo, useRef } from "react";
import { useActiveConversationRows } from "@/features/agent/public/conversation";
import { useActiveSessionId } from "@/features/agent/public/session";
import { Slot } from "@/extensions/host/Slot";
import {
  useExpandedWorkspaceToolIds,
  useToggleWorkspaceTool,
} from "@/features/workspace/public/navigation";
import { useStreamRevealStore } from "@/features/chat/public/message/streamReveal";
import { ChatErrorBoundary } from "@/features/shell/workbench/panel/ChatErrorBoundary";
import { ComposerSurface } from "@/features/shell/workbench/panel/ComposerSurface";
import { FloatingComposer } from "@/features/shell/workbench/panel/FloatingComposer";
import { readingColumn as rc } from "@/features/chat/public/message/rendering";
import { COMPOSER_OVERLAY_PROPERTY } from "@/features/shell/workbench/panel/composerOverlay";
import { CwdMissingBanner } from "@/features/shell/workbench/panel/CwdMissingBanner";
import { MessageStream } from "@/features/shell/workbench/panel/MessageStream";
import { RunErrorBanner } from "@/features/shell/workbench/panel/RunErrorBanner";
import { EmptyChatHeading } from "@/features/shell/workbench/panel/ProjectSelector";
import { pendingQuestionRequest } from "@/features/chat/public/message/rendering";
import { QuestionCard } from "@/features/chat/public/message/rendering";
import { shellStyles as sh } from "@/features/shell/workbench/shellStyles";
import { color, space, type as typeStep, weight } from "@flame/ui/tokens.stylex";
import { vocab } from "@flame/ui";
import { useContentVisible } from "@/ui/index";
const cst = stylex.create({
  hideWhenEmpty: { display: { default: null, ":empty": "none" } },
  queryable: { containerType: "inline-size", "--transcript-jump-gap": space.s2 },
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

  const expandedToolIds = useExpandedWorkspaceToolIds();
  const toggleExpandedTool = useToggleWorkspaceTool();

  const textReveal = useStreamRevealStore((state) => state.streamReveal);

  const pendingQuestion = useMemo(() => pendingQuestionRequest(rows), [rows]);

  const ctx = useMemo(
    () => ({
      sessionId,
      expandedIds: expandedToolIds,
      onToggleExpand: toggleExpandedTool,
      textReveal,
      questionInComposer: pendingQuestion?.block,
    }),
    [sessionId, expandedToolIds, toggleExpandedTool, textReveal, pendingQuestion],
  );

  const composer = pendingQuestion ? (
    <QuestionCard
      // The composer card lives outside the session-keyed transcript, so its own
      // identity key is what resets draft/submit state when the awaiting question
      // changes; without it a prior question's state would bleed into the next.
      key={tupleKey(sessionId, pendingQuestion.resumeRunId, pendingQuestion.itemId)}
      sessionId={sessionId}
      status={pendingQuestion.block.status}
      resumeRunId={pendingQuestion.resumeRunId}
      itemId={pendingQuestion.itemId}
      questions={pendingQuestion.block.questions}
      answered={pendingQuestion.block.answered}
      answers={pendingQuestion.block.answers}
    />
  ) : (
    <ComposerSurface onSend={onSend} />
  );
  const started = rows.length > 0;

  const paneRef = useRef<HTMLDivElement>(null);
  const composerOverlayRef = useRef<HTMLDivElement>(null);

  const visible = useContentVisible();

  useLayoutEffect(() => {
    if (!started || !visible) return;
    const pane = paneRef.current;
    const overlay = composerOverlayRef.current;
    if (!pane || !overlay) return;

    const publishHeight = (height = overlay.getBoundingClientRect().height) => {
      pane.style.setProperty(COMPOSER_OVERLAY_PROPERTY, `${height}px`);
    };

    publishHeight();
    const observer = new ResizeObserver(([entry]) => {
      const borderBox = entry?.borderBoxSize[0];
      publishHeight(borderBox?.blockSize);
    });
    observer.observe(overlay);
    return () => {
      observer.disconnect();
      pane.style.removeProperty(COMPOSER_OVERLAY_PROPERTY);
    };
  }, [started, visible]);

  return (
    <div ref={paneRef} {...stylex.props(sh.paneAnchored, cst.queryable)}>
      <ChatBanners sessionId={sessionId} />
      <div {...stylex.props(sh.paneAnchored, !started && cst.empty)}>
        {started && (
          <>
            <div data-slot="chat-rail" {...stylex.props(cst.rail)}>
              <Slot name="chat.rail.start" />
            </div>
            <div {...stylex.props(sh.paneAnchored)}>
              <ChatErrorBoundary resetKey={sessionId} label={`session:${sessionId}`}>
                <MessageStream rows={rows} ctx={ctx} sessionId={sessionId} />
              </ChatErrorBoundary>
            </div>
          </>
        )}
        {!started && (
          <div {...stylex.props(rc.box, rc.gutter)}>
            <h2 {...stylex.props(cst.heading, typeStep.displayMd)}>
              <EmptyChatHeading />
            </h2>
          </div>
        )}
        <FloatingComposer overlayRef={composerOverlayRef} floating={started}>
          {composer}
        </FloatingComposer>
        {!started && (
          <div {...stylex.props(rc.box, rc.gutter, cst.hideWhenEmpty)}>
            <Slot name="chat.empty" />
          </div>
        )}
      </div>
    </div>
  );
}
