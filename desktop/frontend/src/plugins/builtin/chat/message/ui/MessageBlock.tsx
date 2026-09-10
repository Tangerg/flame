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
import { renderBlock, renderMessageBlocks } from "./BlockRenderer";
import {
  MessageVisibleMaterialOwner,
  MessageVisibleMaterialProvider,
  useVisibleActionMaterialization,
} from "./messageVisibleMaterial";
import { reveal } from "@/ui";
import { corner, type as typeStep } from "@/styles/tokens.stylex";
import { messageStyles } from "./messageStyles";

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

  // Activating something inside a message can take away the thing that was activated. The
  // action bar is removed outright when the message materializes again —
  // `messageActionsVisibility` answers "absent" for that, correctly, because a message being
  // rebuilt has nothing to act on — and an approval card is removed once it has been answered.
  // Measured on the narrative route: focus Regenerate and press Enter, or Deny and press Enter,
  // and focus is on `<body>`. The node is not disabled, it is gone, so the next Tab restarts at
  // the top of the document instead of continuing from this message.
  //
  // Stated once over the whole column rather than per disappearing part, because the column is
  // what survives and is where the reader already was.
  //
  // Keyed on focus ARRIVING, not on blur. Removing the focused element does not dispatch a blur
  // event — focus just becomes `<body>` silently — so a version of this that listened for
  // `onBlurCapture` never ran at the only moment it was needed, and the audit caught it still
  // reporting the same orphans. What does fire reliably is focus coming in, so that is what is
  // remembered; a blur to a REAL element clears it, since focus moving somewhere on purpose is
  // someone navigating and stealing it back would fight them.
  //
  // The effect has no dependency list on purpose: the render that removes the bar is the render
  // that has to be noticed, and it carries no state of its own to depend on.
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
      {content}
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
          // Programmatic focus only — `-1` keeps it out of the tab order, so the rescue below
          // can put focus here without adding a stop nobody asked for.
          tabIndex={-1}
          onFocusCapture={() => {
            heldFocus.current = true;
          }}
          onBlurCapture={(event) => {
            if (event.relatedTarget) heldFocus.current = false;
          }}
          {...stylex.props(reveal.host, messageStyles.column, isUser && messageStyles.columnUser)}
        >
          {/* `sr-only` is the mechanism `globals.css` owns. `select-none` is not decoration:
              the heading IS in the DOM, so without it the role name lands in copied text. */}
          <h2 className={cn("sr-only", stylex.props(messageStyles.unselectable).className)}>
            {roleLabel}
          </h2>
          {msg.phase === "commentary" ? (
            messageContent
          ) : (
            <MessageContextMenu msg={msg}>{messageContent}</MessageContextMenu>
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

// One fact — how visible the action bar is — in one language. It had been three: a Tailwind
// pair, a StyleX class read out of `stylex.props`, and another Tailwind class, so nothing
// could tell whether `opacity-100` was overriding the reveal channel or agreeing with it.
const ACTIONS_VISIBILITY = {
  hidden: messageStyles.actionsHidden,
  hover: reveal.shown,
  pinned: messageStyles.actionsPinned,
  // `unknown` for the value on purpose: what is checked here is that every state has an
  // answer, and `StyleXStyles` cannot type `reveal.shown` — its `pointer-events` is a custom
  // property, which CSS's own enum for that property does not admit.
} as const satisfies Record<Exclude<MessageActionsVisibility, "absent">, unknown>;
