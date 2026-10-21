import { IconButton } from "@flame/ui";
import { iconGlyph } from "@/ui/icons/index";

export function ReturnButton({ label, onReturn }: { label: string; onReturn: () => void }) {
  return (
    <IconButton
      icon={iconGlyph("arrow-left")}
      size="sm"
      quiet
      aria-label={label}
      data-chrome-focus=""
      onClick={onReturn}
    />
  );
}
