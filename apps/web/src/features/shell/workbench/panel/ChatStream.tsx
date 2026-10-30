import { useComposer } from "@/features/composer/public/composer";
import * as stylex from "@stylexjs/stylex";
import { companion } from "@flame/brand-assets";
import { tupleKey } from "@/lib/tupleKey";
import { useT } from "@/lib/i18n/index";
import { type AgentInput } from "@/features/agent/public/input";
import { type FocusEvent, memo, useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { useActiveConversationRows } from "@/features/agent/public/conversation";
import { useActiveSessionId } from "@/features/agent/public/session";
import { Slot } from "@/extensions/host/Slot";
import {
  useExpandedWorkspaceToolIds,
  useToggleWorkspaceTool,
} from "@/features/workspace/public/navigation";
import { useStreamRevealStore } from "@/features/chat/public/message/streamReveal";
import { focusComposer } from "@/features/composer/public/focus";
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
import { ScrollRegion, vocab } from "@flame/ui";
import { useContentVisible } from "@/ui/index";
import { useAgentShellElement } from "@/ui/agent/index";
import { useEmptyConversationFocus } from "@/features/shell/workbench/panel/useEmptyConversationFocus";
import {
  NOTIFICATION_BOTTOM_PROPERTY,
  NOTIFICATION_TOP_PROPERTY,
  notificationInsets,
} from "@/features/shell/workbench/panel/notificationSpace";
const cst = stylex.create({
  hideWhenEmpty: { display: { default: null, ":empty": "none" } },
  notices: { maxHeight: "40%", paddingBlock: 0 },
  queryable: { containerType: "inline-size", "--transcript-jump-gap": space.s2 },
  emptyQuery: { containerType: "size", containerName: "empty-conversation" },
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
    justifyContent: "safe center",
    gap: space.s5,
    overflowY: "auto",
    overscrollBehavior: "contain",
    scrollPaddingBlock: space.s2,
    paddingBottom: "var(--notification-empty-inset)",
  },
  identity: {
    display: "flex",
    flexDirection: "column",
    gap: {
      default: space.s4,
      "@container empty-conversation (max-height: 30rem)": space.s3,
    },
  },
  companion: {
    display: "block",
    alignSelf: "center",
    flexShrink: 0,
    width: {
      default: space.s24,
      "@container empty-conversation (max-height: 30rem)": space.s16,
    },
    height: {
      default: space.s24,
      "@container empty-conversation (max-height: 30rem)": space.s16,
    },
    objectFit: "contain",
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
  const t = useT();
  return (
    <ScrollRegion
      label={t("chat.notices")}
      data-focus-inset=""
      styles={[rc.box, rc.gutter, vocab.hold, cst.notices, cst.hideWhenEmpty]}
    >
      <CwdMissingBanner key={sessionId} />
      <RunErrorBanner />
      <Slot name="chat.banner.top" wrapper {...stylex.props(cst.tray)} />
    </ScrollRegion>
  );
});

export function ChatStream({ onSend }: Props) {
  const composerAccess = useComposer();

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
  const layoutRef = useRef<HTMLDivElement>(null);
  const emptyViewportRef = useRef<HTMLDivElement>(null);
  const composerOverlayRef = useRef<HTMLDivElement>(null);
  const jumpRef = useRef<HTMLDivElement>(null);
  const emptyHeadingRef = useRef<HTMLDivElement>(null);
  const emptyFooterRef = useRef<HTMLDivElement>(null);

  const visible = useContentVisible();
  const shell = useAgentShellElement();

  useLayoutEffect(() => {
    if (!visible) return;
    const pane = paneRef.current;
    const layout = layoutRef.current;
    const overlay = composerOverlayRef.current;
    if (!pane || !layout || !overlay) return;
    const jump = jumpRef.current;

    const publish = () => {
      const overlayRect = overlay.getBoundingClientRect();
      if (started) pane.style.setProperty(COMPOSER_OVERLAY_PROPERTY, `${overlayRect.height}px`);
      if (!shell) return;
      const jumpRect = jump?.getBoundingClientRect();
      const footprint = jumpRect
        ? {
            top: Math.min(overlayRect.top, jumpRect.top),
            bottom: Math.max(overlayRect.bottom, jumpRect.bottom),
          }
        : overlayRect;
      const insets = notificationInsets(
        layout.getBoundingClientRect(),
        footprint,
        document.documentElement.clientHeight,
      );
      shell.style.setProperty(NOTIFICATION_TOP_PROPERTY, `${insets.top}px`);
      shell.style.setProperty(NOTIFICATION_BOTTOM_PROPERTY, `${insets.bottom}px`);
    };

    publish();
    const observer = new ResizeObserver(publish);
    for (const node of [layout, overlay, jump, emptyHeadingRef.current, emptyFooterRef.current]) {
      if (node) observer.observe(node);
    }
    // The empty layout centers and scrolls its siblings, moving the composer without resizing it.
    if (!started) layout.addEventListener("scroll", publish, { passive: true, capture: true });
    return () => {
      observer.disconnect();
      layout.removeEventListener("scroll", publish, true);
      pane.style.removeProperty(COMPOSER_OVERLAY_PROPERTY);
      shell?.style.removeProperty(NOTIFICATION_TOP_PROPERTY);
      shell?.style.removeProperty(NOTIFICATION_BOTTOM_PROPERTY);
    };
  }, [started, visible, shell]);

  useEmptyConversationFocus(emptyViewportRef, !started && visible);

  // Only this host survives the question-to-Composer replacement.
  const questionFocusRef = useRef<{ element: Element; sessionId: string } | null>(null);
  const questionPending = pendingQuestion !== null;

  const rememberQuestionFocus = (event: FocusEvent) => {
    questionFocusRef.current =
      visible && questionPending && composerOverlayRef.current?.contains(event.target)
        ? { element: event.target, sessionId }
        : null;
  };

  const forgetQuestionFocus = (event: FocusEvent) => {
    const next = event.relatedTarget;
    if (next && !composerOverlayRef.current?.contains(next)) questionFocusRef.current = null;
  };

  // Passive so it runs after the new Composer input registers its focus target.
  useEffect(() => {
    const handoff = questionFocusRef.current;
    if (!handoff) return;
    if (!visible || handoff.sessionId !== sessionId) {
      questionFocusRef.current = null;
      return;
    }
    if (questionPending) return;
    questionFocusRef.current = null;
    if (handoff.element.isConnected || document.activeElement !== document.body) {
      return;
    }
    focusComposer(composerAccess);
  }, [questionPending, sessionId, visible, composerAccess]);

  return (
    <div
      ref={paneRef}
      onFocusCapture={rememberQuestionFocus}
      onBlurCapture={forgetQuestionFocus}
      {...stylex.props(sh.paneAnchored, cst.queryable)}
    >
      <ChatBanners sessionId={sessionId} />
      <div ref={layoutRef} {...stylex.props(sh.paneAnchored, !started && cst.emptyQuery)}>
        <div
          ref={emptyViewportRef}
          data-slot={!started ? "empty-conversation-layout" : undefined}
          {...stylex.props(sh.paneAnchored, !started && cst.empty)}
        >
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
            <div ref={emptyHeadingRef} {...stylex.props(rc.box, rc.gutter)}>
              <div {...stylex.props(cst.identity)}>
                <img
                  src={companion}
                  alt=""
                  width={1024}
                  height={1024}
                  draggable={false}
                  {...stylex.props(cst.companion)}
                />
                <h2 {...stylex.props(cst.heading, typeStep.displayMd)}>
                  <EmptyChatHeading />
                </h2>
              </div>
            </div>
          )}
          <FloatingComposer overlayRef={composerOverlayRef} jumpRef={jumpRef} floating={started}>
            {composer}
          </FloatingComposer>
          {!started && (
            <div ref={emptyFooterRef} {...stylex.props(rc.box, rc.gutter, cst.hideWhenEmpty)}>
              <Slot name="chat.empty" />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
