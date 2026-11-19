import { useEffect, useState } from "react";
import { useComposer } from "@/features/composer/public/composer";
import { DropdownMenu } from "@/ui/atoms/menu";
import { useT } from "@/lib/i18n/index";
import { captureComposerSelectionQuote } from "@/features/composer/public/selectionQuote";
interface QuoteIntent {
  readonly x: number;
  readonly y: number;
  readonly append: () => boolean;
}
export function SelectionQuote() {
  const composer = useComposer();
  const t = useT();
  const [intent, setIntent] = useState<QuoteIntent | null>(null);
  useEffect(() => {
    const open = (event: MouseEvent) => {
      if (
        event.defaultPrevented ||
        (event.target instanceof Element &&
          event.target.closest("input,textarea,[contenteditable]"))
      )
        return;
      const append = captureComposerSelectionQuote(composer);
      if (!append) return;
      event.preventDefault();
      setIntent(
        Object.freeze({
          x: event.clientX,
          y: event.clientY,
          append,
        }),
      );
    };
    document.addEventListener("contextmenu", open);
    return () => document.removeEventListener("contextmenu", open);
  }, [composer]);
  if (!intent) return null;
  return (
    <DropdownMenu.Root
      open
      onOpenChange={(open) => {
        if (!open) setIntent(null);
      }}
    >
      <DropdownMenu.Content
        anchor={{ getBoundingClientRect: () => new DOMRect(intent.x, intent.y, 0, 0) }}
        finalFocus={false}
      >
        <DropdownMenu.Item
          onClick={() => {
            intent.append();
            setIntent(null);
          }}
        >
          {t("quote.selection")}
        </DropdownMenu.Item>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}
