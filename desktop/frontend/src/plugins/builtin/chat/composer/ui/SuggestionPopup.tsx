import * as stylex from "@stylexjs/stylex";
import { type ReactNode, type RefObject, useEffect } from "react";
import { Popover, SectionLabel } from "@/ui";
import { SUGGESTION_LISTBOX_ID, suggestionOptionId } from "../application/suggestions";
import { type as typeStep } from "@/styles/tokens.stylex";
import { suggestionStyles } from "./suggestionStyles";

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
  useEffect(() => {
    if (!open) return;
    document.getElementById(suggestionOptionId(index))?.scrollIntoView({ block: "nearest" });
  }, [open, index]);
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
