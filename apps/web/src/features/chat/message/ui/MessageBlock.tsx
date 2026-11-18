import * as stylex from "@stylexjs/stylex";
import { AnimatePresence } from "motion/react";
import { type BlockCtx } from "@/features/chat/message/ui/BlockRenderer";
import { type TranscriptRow } from "@/features/agent/public/conversation";
import {
  memo,
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Slot } from "@/extensions/host/Slot";
import { MessageContext } from "@/features/chat/public/messageContext";
import {
  editMessageInComposer,
  messageActionsVisibility,
  type MessageActionsVisibility,
} from "@/features/chat/message-actions/public/messageActions";
import { useComposer } from "@/features/composer/public/composer";
import { useNotifications } from "@/features/notifications/public/notifier";
import { messageActionMaterialization } from "@/features/chat/message/application/messageBlockModel";
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
import { ConfirmDialog, reveal, useContentVisible } from "@/ui/index";
import { Badge, quoteSourceProps } from "@flame/ui";
import { corner, type as typeStep } from "@flame/ui/tokens.stylex";
import { messageStyles } from "@/features/chat/message/ui/messageStyles";
import { UserMessageFold } from "@/features/chat/message/ui/UserMessageFold";
import { SteerInputStatus } from "@/features/chat/message/ui/SteerInputStatus";

interface DraftReplacement {
  status: "confirming" | "dismissed" | "confirmed" | "ready";
  origin: HTMLElement | null;
  replace: () => boolean;
}

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
  const composer = useComposer();
  const { messages } = useNotifications();
  const visible = useContentVisible();
  const columnRef = useRef<HTMLDivElement>(null);
  const [draftReplacement, setDraftReplacement] = useState<DraftReplacement | null>(null);
  useLayoutEffect(() => {
    if (draftReplacement?.status !== "ready") return;
    // The closing dialog and its focus manager must be gone before presenting Composer.
    setDraftReplacement(null);
    if (draftReplacement.replace()) return;
    const origin = draftReplacement.origin?.isConnected
      ? draftReplacement.origin
      : columnRef.current;
    if (visible && origin && origin.ownerDocument.activeElement === origin.ownerDocument.body)
      origin.focus({ preventScroll: true });
    messages.notify("info", t("msgActions.replaceDraft.changed"), { source: "composer" });
  }, [draftReplacement, messages, t, visible]);
  const editInComposer = useCallback(
    (origin?: HTMLElement) => {
      editMessageInComposer(composer, msg, (replace) => {
        setDraftReplacement({ status: "confirming", origin: origin ?? columnRef.current, replace });
      });
    },
    [composer, msg],
  );
  const messageContext = useMemo(
    () => ({ sessionId, message: msg, editInComposer }),
    [sessionId, msg, editInComposer],
  );

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

  const content = renderMessageBlocks(row, ctx, answerFollows);

  const roleLabel = t(isUser ? "role.user" : "role.assistant");

  if (content.length === 0) return terminalFooter ?? null;

  const messageContent = (
    <div
      data-user-message-bubble={isUser ? "" : undefined}
      {...MESSAGE_QUOTE_SOURCE}
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

  const messageColumn = (
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
      {messageContent}
      {isUser && <SteerInputStatus feedback={row.inputFeedback} messageId={msg.id} />}
      {isUser && !msg.steer && msg.runId === null && <Badge>{t("agent.inputNotApplied")}</Badge>}
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
  );

  return (
    <MessageContext.Provider value={messageContext}>
      <MessageVisibleMaterialProvider
        owner={visibleMaterialOwner}
        generation={visibleMaterialGeneration}
      >
        {msg.phase === "commentary" ? (
          messageColumn
        ) : (
          <MessageContextMenu msg={msg} label={roleLabel}>
            {messageColumn}
          </MessageContextMenu>
        )}
        {actionMaterialization === "settled" && terminalFooter}
        {draftReplacement && draftReplacement.status !== "ready" && (
          <ConfirmDialog
            open={visible && draftReplacement.status === "confirming"}
            onOpenChange={(open) => {
              if (!open && draftReplacement.status === "confirming")
                setDraftReplacement({ ...draftReplacement, status: "dismissed" });
            }}
            onOpenChangeComplete={(open) => {
              if (open) return;
              setDraftReplacement(
                draftReplacement.status === "confirmed"
                  ? { ...draftReplacement, status: "ready" }
                  : null,
              );
            }}
            title={t("msgActions.replaceDraft.title")}
            body={t("msgActions.replaceDraft.body")}
            confirmLabel={t("msgActions.replaceDraft.confirm")}
            cancelLabel={t("common.cancel")}
            destructive
            finalFocus={
              draftReplacement.status === "confirmed"
                ? false
                : () =>
                    draftReplacement.origin?.isConnected
                      ? draftReplacement.origin
                      : (columnRef.current ?? false)
            }
            onConfirm={() => setDraftReplacement({ ...draftReplacement, status: "confirmed" })}
          />
        )}
      </MessageVisibleMaterialProvider>
    </MessageContext.Provider>
  );
}

export const MessageBlock = memo(MessageBlockInner);

const MESSAGE_QUOTE_SOURCE = quoteSourceProps({ kind: "message" });

const ACTIONS_VISIBILITY = {
  hidden: messageStyles.actionsHidden,
  hover: reveal.shown,
  pinned: messageStyles.actionsPinned,
} as const satisfies Record<Exclude<MessageActionsVisibility, "absent">, unknown>;
