import { type BaseUIEvent } from "@base-ui/react/types";
import { isImeKey } from "@flame/ui";
import { type KeyboardEvent } from "react";

function revealOffscreenOption(option: HTMLElement, viewport: HTMLDivElement): boolean {
  const bounds = option.getBoundingClientRect();
  const top = viewport.getBoundingClientRect().top + viewport.clientTop;
  const bottom = top + viewport.clientHeight;
  if (!bounds.height || (bounds.bottom > top && bounds.top < bottom)) return false;
  const delta =
    bounds.top < top ? bounds.top - top : Math.min(bounds.bottom - bottom, bounds.top - top);
  viewport.scrollTo({ top: viewport.scrollTop + delta, behavior: "instant" });
  return true;
}

export function guardComboboxEnter(
  event: BaseUIEvent<KeyboardEvent<HTMLInputElement>>,
  viewport: HTMLDivElement | null,
): void {
  if (isImeKey(event.nativeEvent)) {
    event.preventBaseUIHandler();
    return;
  }
  if (
    event.key !== "Enter" ||
    event.defaultPrevented ||
    !viewport?.clientHeight ||
    event.ctrlKey ||
    event.shiftKey ||
    event.altKey ||
    event.metaKey
  )
    return;
  const input = event.currentTarget;
  const id = input.getAttribute("aria-activedescendant");
  const option = id ? input.ownerDocument.getElementById(id) : null;
  if (!option || !viewport.contains(option)) return;
  if (!event.repeat && !revealOffscreenOption(option, viewport)) return;
  event.preventBaseUIHandler();
  event.preventDefault();
  event.stopPropagation();
}
