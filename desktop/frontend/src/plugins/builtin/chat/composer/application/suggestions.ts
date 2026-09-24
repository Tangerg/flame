import { useCallback, useState } from "react";

export const SUGGESTION_LISTBOX_ID = "composer-suggestion-listbox";

export function suggestionOptionId(index: number): string {
  return `composer-suggestion-option-${index}`;
}

export interface SuggestionList<T> {
  open: boolean;
  items: readonly T[];
  index: number;
  setIndex: (index: number) => void;
  accept: (item: T) => void;
  dismiss: () => void;
}

export function handleSuggestionKey<T>(
  list: SuggestionList<T>,
  event: { key: string; shiftKey: boolean },
): boolean {
  if (!list.open) return false;
  if (event.key === "Escape") {
    list.dismiss();
    return true;
  }
  const count = list.items.length;
  if (count === 0) return false;
  switch (event.key) {
    case "ArrowDown":
      list.setIndex((list.index + 1) % count);
      return true;
    case "ArrowUp":
      list.setIndex((list.index - 1 + count) % count);
      return true;
    case "Tab":
      list.accept(list.items[list.index] ?? list.items[0]!);
      return true;
    case "Enter":
      if (event.shiftKey) return false;
      list.accept(list.items[list.index] ?? list.items[0]!);
      return true;
    default:
      return false;
  }
}

export function useSuggestionIndex(candidateKey: string, count: number) {
  const [selection, setSelection] = useState<{ candidateKey: string; index: number } | null>(null);
  const index =
    selection?.candidateKey === candidateKey && selection.index < count ? selection.index : 0;
  const setIndex = useCallback(
    (next: number) => {
      if (next < 0 || next >= count) return;
      setSelection({ candidateKey, index: next });
    },
    [candidateKey, count],
  );
  return { index, setIndex };
}
