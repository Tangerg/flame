import * as stylex from "@stylexjs/stylex";
import { Button, Icon, Tooltip, vocab } from "@flame/ui";
import { iconGlyph } from "@/ui/icons/index";

const styles = stylex.create({
  label: { minWidth: 0, maxInlineSize: "min(24ch, 40vw)" },
});

export function ReturnButton({
  label,
  onReturn,
  showLabel = false,
}: {
  label: string;
  onReturn: () => void;
  showLabel?: boolean;
}) {
  return (
    <Tooltip label={label}>
      <Button
        size={showLabel ? "sm" : "icon-sm"}
        quiet
        aria-label={label}
        data-chrome-focus=""
        onClick={onReturn}
      >
        <Icon glyph={iconGlyph("arrow-left")} size="sm" />
        {showLabel && <span {...stylex.props(styles.label, vocab.truncate)}>{label}</span>}
      </Button>
    </Tooltip>
  );
}
