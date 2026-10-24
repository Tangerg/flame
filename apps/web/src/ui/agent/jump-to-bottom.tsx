import * as stylex from "@stylexjs/stylex";
import { type Ref } from "react";
import { IconButton } from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import { useT } from "@/lib/i18n/index";
import { iconGlyph } from "@/ui/icons/index";
type JumpDockPlacement = "above-composer" | "inside-reader";
const styles = stylex.create({
  dock: {
    position: "absolute",
    left: "50%",
    translate: "-50% 0",
    zIndex: 3,
    pointerEvents: "none",
  },
  aboveComposer: { bottom: "calc(100% + var(--transcript-jump-gap))" },
  insideReader: { bottom: "var(--transcript-jump-gap)" },
  shown: { translate: "0 0", opacity: 1, pointerEvents: "auto" },
  hidden: { translate: `0 ${space.s1}`, opacity: 0, pointerEvents: "none" },
});

export function AgentJumpToBottom({
  visible,
  onClick,
  placement,
  dockRef,
}: {
  visible: boolean;
  onClick: () => void;
  placement: JumpDockPlacement;
  dockRef?: Ref<HTMLDivElement>;
}) {
  const t = useT();
  const label = t("chat.jumpToBottom");
  return (
    <div
      ref={dockRef}
      {...stylex.props(
        styles.dock,
        placement === "above-composer" ? styles.aboveComposer : styles.insideReader,
      )}
    >
      <IconButton
        type="button"
        icon={iconGlyph("chevron-down")}
        variant="raised"
        round
        size="md"
        title={label}
        aria-label={label}
        aria-hidden={!visible}
        onClick={onClick}
        tabIndex={visible ? 0 : -1}
        className={stylex.props(visible ? styles.shown : styles.hidden).className}
      />
    </div>
  );
}
