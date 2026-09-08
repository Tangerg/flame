import * as stylex from "@stylexjs/stylex";
import { scrollStreamToBottom, useStreamAtBottom } from "./streamFollow";
import { IconButton } from "@/ui";
import { useT } from "@/lib/i18n";
import { space } from "@/styles/tokens.stylex";

/**
 * The centring and the entrance are ONE `translate`.
 *
 * Under Tailwind they were two utilities — `-translate-x-1/2` and `translate-y-{0,1}` — which
 * compose only because each writes its own custom property. In CSS `translate` is a single
 * property: two declarations do not merge, the later one wins, and the button loses the half
 * of its own width that was centring it. So each state states both axes.
 */
const styles = stylex.create({
  // No transition here. `Button` already names every property a button animates and says why
  // at its own declaration: the list is the whole list, so a call site restating a SUBSET of
  // it does not narrow the transition to what it cares about — it silently drops the rest.
  // This one asked for `opacity, translate` and took the press scale and every colour with it.
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
