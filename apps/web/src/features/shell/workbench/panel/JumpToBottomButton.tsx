import * as stylex from "@stylexjs/stylex";
import {
  scrollStreamToBottom,
  useStreamAtBottom,
} from "@/features/shell/workbench/panel/streamFollow";
import { AgentJumpToBottom } from "@/ui/agent/index";
const styles = stylex.create({
  float: {
    position: "absolute",
    left: "50%",
    bottom: "calc(100% + var(--transcript-jump-gap))",
    translate: "-50% 0",
    zIndex: 3,
    pointerEvents: "none",
  },
});

export function JumpToBottomButton() {
  const visible = !useStreamAtBottom();
  return (
    <div {...stylex.props(styles.float)}>
      <AgentJumpToBottom visible={visible} onClick={scrollStreamToBottom} />
    </div>
  );
}
