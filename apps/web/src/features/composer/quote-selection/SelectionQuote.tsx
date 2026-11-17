import { useEffect, useState } from "react";
import { useComposer } from "@/features/composer/public/composer";
import { DropdownMenu } from "@/ui/atoms/menu";
import { useT } from "@/lib/i18n/index";
import { captureQuote } from "./captureQuote";
interface QuoteIntent {
  readonly x: number;
  readonly y: number;
  readonly text: string;
  readonly append: (text: string) => boolean;
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
      const text = captureQuote();
      if (!text) return;
      event.preventDefault();
      setIntent(
        Object.freeze({
          x: event.clientX,
          y: event.clientY,
          text,
          append: composer.captureAppendText(),
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
            intent.append(intent.text);
            setIntent(null);
          }}
        >
          {t("quote.selection")}
        </DropdownMenu.Item>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}
