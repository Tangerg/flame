import * as stylex from "@stylexjs/stylex";
import type { BlockCtx } from "./BlockRenderer";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import { memo, useEffect, useMemo, useRef, type ReactNode } from "react";
import { Slot } from "@/plugins/host/Slot";
import { MessageContext } from "@/plugins/sdk/messageContext";
import {
  messageActionsVisibility,
  type MessageActionsVisibility,
} from "@/plugins/builtin/chat/message-actions/public/messageActions";
import {
  messageActionMaterialization,
  messageBlocksRenderInstant,
} from "../application/messageBlockModel";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { MESSAGE_CONTENT_CLASS } from "./messageContent";
import { MessageContextMenu } from "./MessageContextMenu";
import { AnimatePresence } from "motion/react";
import { renderBlock, renderMessageBlocks } from "./BlockRenderer";
import {
  MessageVisibleMaterialOwner,
  MessageVisibleMaterialProvider,
  useVisibleActionMaterialization,
} from "./messageVisibleMaterial";
import { Badge, reveal } from "@/ui";
import { corner, type as typeStep } from "@/styles/tokens.stylex";
import { messageStyles } from "./messageStyles";
import { UserMessageFold } from "./UserMessageFold";

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
  useEffect(() => {
    if (!heldFocus.current) return;
    if (document.activeElement !== document.body) return;
    heldFocus.current = false;
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

  if (content.length === 0) return null;

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
          onFocusCapture={() => {
            heldFocus.current = true;
          }}
          onBlurCapture={(event) => {
            if (event.relatedTarget) heldFocus.current = false;
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
          {isUser && msg.runId === null && <Badge>{t("agent.inputNotApplied")}</Badge>}
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
