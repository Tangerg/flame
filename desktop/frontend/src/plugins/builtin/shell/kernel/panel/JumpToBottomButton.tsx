import { scrollStreamToBottom, useStreamAtBottom } from "./streamFollow";
import { IconButton } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";

export function JumpToBottomButton() {
  const t = useT();
  const visible = !useStreamAtBottom();
  const label = t("chat.jumpToBottom");
  return (
    <IconButton
      type="button"
      icon="chevron-down"
      size="md"
      title={label}
      aria-label={label}
      onClick={scrollStreamToBottom}
      tabIndex={visible ? 0 : -1}
      className={cn(
        "absolute bottom-[calc(100%+0.5rem)] left-1/2 -translate-x-1/2 z-3 grid h-8 w-8 place-items-center rounded-full",
        "bg-canvas text-fg-soft border-0",
        "shadow-[var(--shadow-raised)] transition-[opacity,translate,scale,background] duration-[var(--dur-fast)]",
        "hover:bg-surface-2 hover:text-fg",
        "active:translate-y-0 active:scale-[var(--press-scale)]",
        visible
          ? "opacity-100 translate-y-0 pointer-events-auto"
          : "opacity-0 translate-y-1 pointer-events-none",
      )}
    />
  );
}
