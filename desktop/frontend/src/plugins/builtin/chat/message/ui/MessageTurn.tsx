import * as stylex from "@stylexjs/stylex";
import { motion } from "motion/react";
import type { ComponentProps } from "react";
import { space } from "@flame/ui/tokens.stylex";
import { cn } from "@/lib/classNames";
import { enterUp } from "@/lib/motion";
import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { MessageBlock } from "./MessageBlock";
import { finalAnswerFollows } from "../application/messageBlockModel";

const styles = stylex.create({
  turn: { position: "relative" },
  continuation: { marginTop: space.s2 },
  next: { marginTop: space.s6 },
});

export function MessageTurn({
  previous,
  next,
  className,
  ...props
}: Omit<ComponentProps<typeof MessageBlock>, "answerFollows"> & {
  previous?: Message;
  next?: Message;
  className?: string;
}) {
  const { message } = props.row;
  const continuation =
    previous &&
    message.phase !== "finalAnswer" &&
    previous.role === message.role &&
    previous.runId === message.runId;
  return (
    <motion.div
      {...enterUp}
      data-turn-id={message.id}
      data-turn-role={message.role}
      className={cn(
        stylex.props(styles.turn, previous && (continuation ? styles.continuation : styles.next))
          .className,
        className,
      )}
    >
      <MessageBlock {...props} answerFollows={finalAnswerFollows(message, next)} />
    </motion.div>
  );
}
