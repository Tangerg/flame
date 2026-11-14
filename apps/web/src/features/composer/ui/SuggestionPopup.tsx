import * as stylex from "@stylexjs/stylex";
import { type ReactNode, type RefObject, useLayoutEffect } from "react";
import { Popover } from "@/ui/index";
import { SectionLabel } from "@/ui/index";
import { type SuggestionNavigation } from "@/features/composer/ui/useSuggestionNavigation";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { suggestionStyles } from "@/features/composer/ui/suggestionStyles";
interface Props {
  open: boolean;
  heading: string;
  navigation: SuggestionNavigation;
  onDismiss: () => void;
  anchor: RefObject<HTMLElement | null>;
  children: ReactNode;
  status?: ReactNode;
}

export function SuggestionPopup({
  open,
  heading,
  navigation,
  onDismiss,
  anchor,
  children,
  status,
}: Props) {
  const { candidateKey, activeIndex, items, list, setList, getFloatingProps } = navigation;
  useLayoutEffect(() => {
    if (!open || !list) return;
    const reveal = () => {
      const item = activeIndex === null ? null : items.current[activeIndex];
      if (!item || !list.contains(item)) return;
      const bounds = list.getBoundingClientRect();
      const row = item.getBoundingClientRect();
      const top = bounds.top + list.clientTop;
      const bottom = top + list.clientHeight;
      // Native offsets can quantize fractions; round toward fully revealing the row.
      if (row.top < top) list.scrollTop -= Math.ceil(top - row.top);
      else if (row.bottom > bottom) list.scrollTop += Math.ceil(row.bottom - bottom);
    };
    const observer = new ResizeObserver(reveal);
    observer.observe(list);
    const item = activeIndex === null ? null : items.current[activeIndex];
    if (item) observer.observe(item);
    const frame = list.ownerDocument.defaultView!.requestAnimationFrame(reveal);
    return () => {
      observer.disconnect();
      list.ownerDocument.defaultView!.cancelAnimationFrame(frame);
    };
  }, [open, candidateKey, activeIndex, items, list]);
  return (
    <Popover.Anchored
      open={open}
      onOpenChange={(next) => {
        if (!next) onDismiss();
      }}
      anchor={anchor}
      aria-label={heading}
      {...stylex.props(suggestionStyles.panel)}
    >
      <SectionLabel {...stylex.props(suggestionStyles.heading)}>{heading}</SectionLabel>
      {status === undefined ? (
        <div
          ref={setList}
          {...getFloatingProps()}
          aria-label={heading}
          {...stylex.props(suggestionStyles.list)}
        >
          {children}
        </div>
      ) : (
        <div role="status" {...stylex.props(suggestionStyles.state, typeStep.uiSm)}>
          {status}
        </div>
      )}
    </Popover.Anchored>
  );
}
