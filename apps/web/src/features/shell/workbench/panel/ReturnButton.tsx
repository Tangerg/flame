import { Button, Icon, Tooltip } from "@flame/ui";
import { iconGlyph } from "@/ui/icons/index";

export function ReturnButton({ label, onReturn }: { label: string; onReturn: () => void }) {
  return (
    <Tooltip label={label}>
      <Button size="icon-sm" quiet aria-label={label} data-chrome-focus="" onClick={onReturn}>
        <Icon glyph={iconGlyph("arrow-left")} size="sm" />
      </Button>
    </Tooltip>
  );
}
