import * as stylex from "@stylexjs/stylex";
import { type ReactNode, type RefObject, useCallback } from "react";
import { Popover } from "@/ui/index";
import { SectionLabel } from "@/ui/index";
import {
  SUGGESTION_LISTBOX_ID,
  suggestionOptionId,
} from "@/features/composer/application/suggestions";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { suggestionStyles } from "@/features/composer/ui/suggestionStyles";
interface Props {
  open: boolean;
  heading: string;
  index: number;
  onDismiss: () => void;
  anchor: RefObject<HTMLElement | null>;
  children: ReactNode;
  status?: ReactNode;
}

export function SuggestionPopup({
  open,
  heading,
  index,
  onDismiss,
  anchor,
  children,
  status,
}: Props) {
  // The portal mounts and Base UI constrains its height after the parent
  // commits, so reveal the selection from the list's own lifetime and size.
  const revealSelected = useCallback(
    (list: HTMLDivElement | null) => {
      if (!open || !list) return;
      const observer = new ResizeObserver(() => {
        list.querySelector(`#${suggestionOptionId(index)}`)?.scrollIntoView({ block: "nearest" });
      });
      observer.observe(list);
      return () => observer.disconnect();
    },
    [open, index],
  );
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
          ref={revealSelected}
          id={SUGGESTION_LISTBOX_ID}
          role="listbox"
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
