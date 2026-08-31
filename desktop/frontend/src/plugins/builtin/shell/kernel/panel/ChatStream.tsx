import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import { memo, useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { useActiveConversationRows } from "@/plugins/builtin/agent/public/conversation";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { cn } from "@/lib/classNames";
import { Slot } from "@/plugins/host/Slot";
import {
  reconcileWorkspaceToolSelection,
  useExpandedWorkspaceToolIds,
  useSelectWorkspaceTool,
  useToggleWorkspaceTool,
} from "@/plugins/builtin/workspace/public/navigation";
import { useUiStore } from "@/state/uiStore";
import { ChatErrorBoundary } from "./ChatErrorBoundary";
import { ComposerSurface } from "./ComposerSurface";
import { ComposerOverlayTop, FloatingComposer, RuntimeConnectionNotice } from "./FloatingComposer";
import { COMPOSER_OVERLAY_PROPERTY, READING_COLUMN, READING_GUTTER } from "./readingColumn";
import { CwdMissingBanner } from "./CwdMissingBanner";
import { MessageStream, type MessageStreamController } from "./MessageStream";
import { RunErrorBanner } from "./RunErrorBanner";
import { EmptyChatHeading } from "./ProjectSelector";
import {
  pendingQuestionRequest,
  QuestionCard,
} from "@/plugins/builtin/chat/message/public/rendering";

interface Props {
  onSend: (input: AgentInput) => boolean;
}

const RAIL =
  "absolute top-0 bottom-[var(--composer-overlay,0px)] z-1 hidden w-[var(--reading-rail-width)] flex-col @min-[1152px]:flex pointer-events-none [&>*]:pointer-events-auto right-[calc(50%+var(--reading-column-max)/2)]";

const ChatBanners = memo(function ChatBanners({ sessionId }: { sessionId: string }) {
  return (
    <div className={cn(READING_COLUMN, READING_GUTTER, "shrink-0")}>
      <CwdMissingBanner key={sessionId} />
      <RunErrorBanner />
      <Slot
        name="chat.banner.top"
        wrapper
        className="pointer-events-auto flex flex-col gap-1.5 py-1.5"
      />
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

  const textReveal = useUiStore((state) => state.streamReveal);

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
    return (
      <>
        <ChatBanners sessionId={sessionId} />
        <div className="panel-scroll flex flex-1 flex-col items-center justify-center gap-5 pb-[6vh]">
          <div className={cn(READING_COLUMN, READING_GUTTER)}>
            <h1 className="mx-auto max-w-[620px] text-balance text-center text-display-md font-medium text-fg">
              <EmptyChatHeading />
            </h1>
          </div>
          <div className={cn(READING_COLUMN, READING_GUTTER)}>
            <ComposerOverlayTop />
            <RuntimeConnectionNotice />
            {composer}
          </div>
          <div className={cn(READING_COLUMN, READING_GUTTER, "empty:hidden")}>
            <Slot name="chat.empty" />
          </div>
        </div>
      </>
    );
  }

  return (
    <div ref={paneRef} className="@container relative flex min-h-0 flex-1 flex-col">
      <ChatBanners sessionId={sessionId} />
      <div className="relative flex min-h-0 flex-1 flex-col">
        <div className={RAIL}>
          <Slot name="chat.rail.start" />
        </div>
        <div className="relative flex min-h-0 flex-1 flex-col">
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
