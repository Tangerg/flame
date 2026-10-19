import * as stylex from "@stylexjs/stylex";
import { IconButton } from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import { useT } from "@/lib/i18n";
import { iconGlyph } from "@/ui/icons";

const styles = stylex.create({
  shown: { translate: "0 0", opacity: 1, pointerEvents: "auto" },
  hidden: { translate: `0 ${space.s1}`, opacity: 0, pointerEvents: "none" },
});

export function AgentJumpToBottom({ visible, onClick }: { visible: boolean; onClick: () => void }) {
  const t = useT();
  const label = t("chat.jumpToBottom");
  return (
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
  );
}
