import * as stylex from "@stylexjs/stylex";
import { AnimatePresence } from "motion/react";
import { type BlockCtx } from "@/features/chat/message/ui/BlockRenderer";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import { memo, useEffect, useMemo, useRef, type ReactNode } from "react";
import { Slot } from "@/extensions/host/Slot";
import { MessageContext } from "@/features/chat/public/messageContext";
import {
  messageActionsVisibility,
  type MessageActionsVisibility,
} from "@/features/chat/message-actions/public/messageActions";
import {
  messageActionMaterialization,
  messageBlocksRenderInstant,
} from "@/features/chat/message/application/messageBlockModel";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n/index";
import { MESSAGE_CONTENT_CLASS } from "@/features/chat/message/ui/messageContent";
import { MessageContextMenu } from "@/features/chat/message/ui/MessageContextMenu";
import { renderBlock, renderMessageBlocks } from "@/features/chat/message/ui/BlockRenderer";
import {
  MessageVisibleMaterialOwner,
  MessageVisibleMaterialProvider,
  useVisibleActionMaterialization,
} from "@/features/chat/message/ui/messageVisibleMaterial";
import { reveal } from "@/ui/index";
import { Badge } from "@flame/ui";
import { corner, type as typeStep } from "@flame/ui/tokens.stylex";
import { messageStyles } from "@/features/chat/message/ui/messageStyles";
import { UserMessageFold } from "@/features/chat/message/ui/UserMessageFold";
import { SteerInputStatus } from "@/features/chat/message/ui/SteerInputStatus";
function MessageBlockInner({
  row,
  ctx,
  sessionId,
  isLast,
  isRunning,
  answerFollows = false,
  terminalFooter,
}: {
  row: TranscriptRow;
  ctx: BlockCtx;
  sessionId: string;
  isLast: boolean;
  isRunning: boolean;
  answerFollows?: boolean;
  terminalFooter?: ReactNode;
}) {
  const msg = row.message;
  const isUser = msg.role === "user";
  const t = useT();
  const messageContext = useMemo(() => ({ sessionId, message: msg }), [sessionId, msg]);

  const visibleMaterialOwner = useMemo(
    () => new MessageVisibleMaterialOwner(sessionId, msg.id),
    [msg.id, sessionId],
  );
  const acceptedActionMaterialization = messageActionMaterialization(row);
  const visibleMaterialGeneration =
    acceptedActionMaterialization === "active" ? visibleMaterialOwner : row;
  const actionMaterialization = useVisibleActionMaterialization(
    visibleMaterialOwner,
    acceptedActionMaterialization,
    visibleMaterialGeneration,
  );
  const actionsVisibility =
    msg.phase === "commentary"
      ? "absent"
      : messageActionsVisibility({
          materialization: actionMaterialization,
          isRunning,
          isLast,
        });

  const columnRef = useRef<HTMLDivElement | null>(null);
  const heldFocus = useRef(false);
  const lastFocused = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (!heldFocus.current) return;
    if (document.activeElement !== document.body) return;
    const prior = lastFocused.current;
    heldFocus.current = false;
    lastFocused.current = null;
    // Reclaim only the focus the stream took: a child that had focus and is now
    // unmounted. One still in the DOM means the user blurred to body on purpose.
    if (prior?.isConnected) return;
    columnRef.current?.focus({ preventScroll: true });
  });

  if (msg.role === "system") {
    return (
      <MessageContext.Provider value={messageContext}>
        <div className={MESSAGE_CONTENT_CLASS}>
          {msg.blocks.map((block, index) => renderBlock(block, index, row.facts, ctx))}
        </div>
      </MessageContext.Provider>
    );
  }

  const blockCtx: BlockCtx = messageBlocksRenderInstant(msg.role)
    ? { ...ctx, textReveal: "instant" }
    : ctx;

  const content = renderMessageBlocks(row, blockCtx, answerFollows);

  const roleLabel = t(isUser ? "role.user" : "role.assistant");

  if (content.length === 0) return terminalFooter ?? null;

  const messageContent = (
    <div
      data-user-message-bubble={isUser ? "" : undefined}
      data-quote-source="message"
      className={cn(
        MESSAGE_CONTENT_CLASS,
        stylex.props(
          messageStyles.body,
          typeStep.prose,
          isUser && messageStyles.bubble,
          isUser && corner.bubble,
        ).className,
      )}
    >
      {isUser ? (
        <UserMessageFold>
          <AnimatePresence initial={false}>{content}</AnimatePresence>
        </UserMessageFold>
      ) : (
        <AnimatePresence initial={false}>{content}</AnimatePresence>
      )}
    </div>
  );

  return (
    <MessageContext.Provider value={messageContext}>
      <MessageVisibleMaterialProvider
        owner={visibleMaterialOwner}
        generation={visibleMaterialGeneration}
      >
        <div
          ref={columnRef}
          tabIndex={-1}
          onFocusCapture={(event) => {
            heldFocus.current = true;
            lastFocused.current = event.target as HTMLElement;
          }}
          onBlurCapture={(event) => {
            if (event.relatedTarget) {
              heldFocus.current = false;
              lastFocused.current = null;
            }
          }}
          {...stylex.props(reveal.host, messageStyles.column, isUser && messageStyles.columnUser)}
        >
          <h2 className={cn("sr-only", stylex.props(messageStyles.unselectable).className)}>
            {roleLabel}
          </h2>
          {msg.phase === "commentary" ? (
            messageContent
          ) : (
            <MessageContextMenu msg={msg}>{messageContent}</MessageContextMenu>
          )}
          {isUser && <SteerInputStatus feedback={row.inputFeedback} messageId={msg.id} />}
          {isUser && !msg.steer && msg.runId === null && (
            <Badge>{t("agent.inputNotApplied")}</Badge>
          )}
          {actionsVisibility !== "absent" && (
            <div
              data-reveal={actionsVisibility === "hover" ? "hover" : undefined}
              {...stylex.props(
                messageStyles.actions,
                ACTIONS_VISIBILITY[actionsVisibility],
                isUser ? messageStyles.actionsOutdentEnd : messageStyles.actionsOutdentStart,
              )}
            >
              <Slot name="message.actions" />
            </div>
          )}
        </div>
        {actionMaterialization === "settled" && terminalFooter}
      </MessageVisibleMaterialProvider>
    </MessageContext.Provider>
  );
}

export const MessageBlock = memo(MessageBlockInner);

const ACTIONS_VISIBILITY = {
  hidden: messageStyles.actionsHidden,
  hover: reveal.shown,
  pinned: messageStyles.actionsPinned,
} as const satisfies Record<Exclude<MessageActionsVisibility, "absent">, unknown>;
