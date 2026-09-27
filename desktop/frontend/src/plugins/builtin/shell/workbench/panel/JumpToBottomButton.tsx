import * as stylex from "@stylexjs/stylex";
import { scrollStreamToBottom, useStreamAtBottom } from "./streamFollow";
import { IconButton } from "@/ui";
import { useT } from "@/lib/i18n";
import { space } from "@/styles/tokens.stylex";

const styles = stylex.create({
  float: {
    position: "absolute",
    left: "50%",
    bottom: "calc(100% + 0.5rem)",
    zIndex: 3,
  },
  shown: { translate: "-50% 0", opacity: 1, pointerEvents: "auto" },
  hidden: { translate: `-50% ${space.s1}`, opacity: 0, pointerEvents: "none" },
});

export function JumpToBottomButton() {
  const t = useT();
  const visible = !useStreamAtBottom();
  const label = t("chat.jumpToBottom");
  return (
    <IconButton
      type="button"
      icon="chevron-down"
      variant="raised"
      round
      size="md"
      title={label}
      aria-label={label}
      onClick={scrollStreamToBottom}
      tabIndex={visible ? 0 : -1}
      className={stylex.props(styles.float, visible ? styles.shown : styles.hidden).className}
    />
  );
}
