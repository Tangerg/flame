import {
  useFloatingRootContext,
  useInteractions,
  useListNavigation,
  useRole,
} from "@floating-ui/react";
import { useLayoutEffect, useRef, useState, type HTMLAttributes, type MouseEvent } from "react";

export function useSuggestionNavigation({
  open,
  candidateKey,
  count,
  dismiss,
  input,
}: {
  open: boolean;
  candidateKey: string | null;
  count: number;
  dismiss(): void;
  input: HTMLTextAreaElement | null;
}) {
  const [list, setList] = useState<HTMLDivElement | null>(null);
  const items = useRef<Array<HTMLElement | null>>([]);
  const [selection, select] = useState<{ key: string | null; index: number | null }>({
    key: null,
    index: null,
  });
  const activeIndex =
    !open || count === 0
      ? null
      : selection.key === candidateKey && (selection.index === null || selection.index < count)
        ? selection.index
        : 0;
  if (
    open &&
    (selection.key !== candidateKey || (selection.index !== null && selection.index >= count))
  ) {
    select({ key: candidateKey, index: activeIndex });
  }
  useLayoutEffect(() => {
    items.current.length = count;
  }, [count]);
  const context = useFloatingRootContext({
    open,
    onOpenChange: (next) => {
      if (!next) dismiss();
    },
    elements: { reference: input, floating: list },
  });
  const enabled = open && count > 0;
  const navigation = useListNavigation(context, {
    enabled,
    listRef: items,
    activeIndex,
    onNavigate: (index) => {
      if (enabled) select({ key: candidateKey, index });
    },
    virtual: true,
    nested: false,
    loop: true,
    openOnArrowKeyDown: false,
    focusItemOnOpen: false,
    scrollItemIntoView: false,
  });
  const role = useRole(context, { enabled, role: "combobox" });
  const { getReferenceProps, getFloatingProps, getItemProps } = useInteractions([navigation, role]);
  const referenceProps = getReferenceProps() as HTMLAttributes<HTMLTextAreaElement>;
  const getOptionProps = (index: number) => ({
    ...getItemProps({
      active: index === activeIndex,
      selected: index === activeIndex,
      tabIndex: -1,
    }),
    ref: (node: HTMLElement | null) => {
      items.current[index] = node;
    },
    onMouseDown: (event: MouseEvent<HTMLElement>) => event.preventDefault(),
    // Floating's item click moves DOM focus; Composer commands retain the input's virtual focus.
    onClick: undefined,
  });
  return {
    candidateKey,
    activeIndex,
    list,
    setList,
    items,
    referenceProps,
    getFloatingProps,
    getOptionProps,
    navigate: navigation.reference?.onKeyDown,
  };
}

export type SuggestionNavigation = ReturnType<typeof useSuggestionNavigation>;
