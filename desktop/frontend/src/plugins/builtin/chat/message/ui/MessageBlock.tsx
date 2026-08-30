import type { BlockCtx } from "./BlockRenderer";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import { memo, useMemo, type ReactNode } from "react";
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
  /** Flips only at run boundaries, so it never churns this memo per token. */
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

  // Placed after all hooks so rules-of-hooks holds.
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

  // A message whose only material is the pending Question is presented by the composer;
  // leaving an empty wrapper here would preserve the duplicate's rhythm.
  if (content.length === 0) return null;

  const messageContent = (
    <div
      data-user-message-bubble={isUser ? "" : undefined}
      className={cn(
        MESSAGE_CONTENT_CLASS,
        "min-w-0 text-pretty leading-prose text-prose text-fg",
        // A neutral wash, not the accent: a prompt owns a distinct material but is not a
        // selected row or a semantic status.
        isUser && "max-w-[77%] rounded-bubble bg-user-message px-3 py-2",
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
        <div className={cn("group relative flex min-w-0 flex-col gap-2", isUser && "items-end")}>
          <h4 className="sr-only select-none">{roleLabel}</h4>
          {msg.phase === "commentary" ? (
            messageContent
          ) : (
            <MessageContextMenu msg={msg}>{messageContent}</MessageContextMenu>
          )}
          {/* Pulled outward by the button's own optical inset so the first glyph
              lands on the text's edge rather than its box doing so — and outward is a
              different side per role now that a user turn hugs the trailing edge. With
              the inset always on the left, the bar under a right-aligned bubble grew
              leftward and its last glyph sat ~5px inside the text it belongs to. */}
          {actionsVisibility !== "absent" && (
            <div
              className={cn(
                "flex shrink-0 transition-[opacity,visibility] duration-[var(--dur-fast)]",
                ACTIONS_VISIBILITY[actionsVisibility],
                isUser
                  ? "-mr-[calc((var(--control-height-sm)-var(--icon-sm))/2)]"
                  : "-ml-[calc((var(--control-height-sm)-var(--icon-sm))/2)]",
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

// Both props are shaped so the default shallow comparison actually bails out: `row` comes
// from the transcript projection, which reuses rows whose message and facts are unchanged,
// and `ctx` holds no session data at all. Both halves are load-bearing — a turn receives
// context narrowed to its own tool calls, so unrelated deltas cannot invalidate it.
export const MessageBlock = memo(MessageBlockInner);

/**
 * Hover reveal stays in CSS so a hovering pointer never triggers a render. The bar stays IN
 * FLOW, reserving the row it may reveal: `absolute top-full` looks like it saves the gap and
 * does not, because every message's `content-visibility` containment slices anything drawn
 * past its edge.
 */
const ACTIONS_VISIBILITY: Record<Exclude<MessageActionsVisibility, "absent">, string> = {
  // `invisible`, not `pointer-events-none opacity-0`: transparency stops the pointer but
  // not the keyboard, leaving focusable buttons nobody can see or reveal. `visibility`
  // also keeps the reserved row, which `display: none` would not.
  hidden: "invisible opacity-0",
  hover: "opacity-0 group-hover:opacity-100 focus-within:opacity-100",
  pinned: "opacity-100",
};
