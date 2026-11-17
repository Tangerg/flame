import { type RefObject, useRef } from "react";
import { Button, handoffFocus, type ButtonProps } from "@flame/ui";

export function MemoryAction({
  result,
  canRestoreFocus,
  ...props
}: Omit<ButtonProps, "ref"> & {
  result: RefObject<HTMLElement | null>;
  canRestoreFocus(): boolean;
}) {
  const action = useRef<HTMLButtonElement>(null);
  return (
    <Button
      ref={action}
      {...props}
      onClick={(event) => {
        if (canRestoreFocus())
          handoffFocus(event.currentTarget, result.current);
        props.onClick?.(event);
      }}
    />
  );
}
